// Package ui is the whole interface: the gh-dash style list, the PR view and
// the diff view, owned by one root model.
package ui

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/chrismatix/gh-sift/internal/config"
	"github.com/chrismatix/gh-sift/internal/diff"
	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/readstate"
)

type screen int

const (
	screenList screen = iota
	screenPullRequest
)

type pullRequestKey struct {
	repository string
	number     int
}

// pullRequestData is everything fetched for one PR. It stays in memory so
// moving through a stack and back does not wait on the network twice.
type pullRequestData struct {
	detail   *gh.PullRequest
	detailAt time.Time
	loading  bool
	err      error
	files    []diff.File
	// Paths the repository's .gitattributes declares generated (true) or not (false).
	generated   map[string]bool
	diffHead    string
	diffLoading bool
	diffErr     error
}

type toast struct {
	text    string
	failure bool
	until   time.Time
}

type App struct {
	config    config.Config
	keys      keymap
	client    *gh.Client
	theme     Theme
	readState *readstate.Store
	width     int
	height    int
	viewer    string
	// The repository sift was started in, if any: "owner/name" and its root.
	repository     string
	repositoryPath string

	// The PR named on the command line, opened instead of the list.
	initial     pullRequestKey
	initialTab  pullRequestTab
	screen      screen
	list        listScreen
	pullRequest pullRequestScreen
	store       map[pullRequestKey]*pullRequestData

	overlay *overlay
	network network
	// Set while the terminal reports it is not focused; polling waits for focus.
	unfocused bool
	showHelp  bool
	toast     toast
	// Count of running background writes, shown as a spinner in the footer.
	busy         int
	busyText     string
	spinnerFrame int
}

type Options struct {
	Config         config.Config
	Client         *gh.Client
	Repository     string
	RepositoryPath string
	// Open this PR directly instead of the list. OpenTab is the last part
	// of a PR URL ("files", "checks") and picks the tab; empty is the overview.
	OpenRepository string
	OpenNumber     int
	OpenTab        string
}

func New(options Options) (*App, error) {
	keys, err := newKeymap(options.Config.Keybindings)
	if err != nil {
		return nil, err
	}
	var colors *config.ColorTheme
	if options.Config.Theme != nil {
		colors = options.Config.Theme.Colors
	}
	app := &App{
		config:         options.Config,
		keys:           keys,
		client:         options.Client,
		theme:          NewTheme(true, colors),
		readState:      readstate.Open(readstate.DefaultPath()),
		repository:     options.Repository,
		repositoryPath: options.RepositoryPath,
		store:          map[pullRequestKey]*pullRequestData{},
		width:          120,
		height:         35,
	}
	app.list = newListScreen(app)
	app.initial = pullRequestKey{options.OpenRepository, options.OpenNumber}
	app.initialTab = map[string]pullRequestTab{"files": tabFiles, "changes": tabFiles, "checks": tabChecks}[options.OpenTab]
	return app, nil
}

type (
	viewerMsg            string
	tickMsg              time.Time
	pullRequestLoadedMsg struct {
		key    pullRequestKey
		detail *gh.PullRequest
		err    error
	}
	diffLoadedMsg struct {
		key       pullRequestKey
		head      string
		files     []diff.File
		generated map[string]bool
		err       error
	}
	// actionMsg reports a finished write; then runs on success to refresh.
	actionMsg struct {
		success string
		err     error
		then    func(app *App) tea.Cmd
	}
)

func (app *App) Init() tea.Cmd {
	commands := []tea.Cmd{
		tea.RequestBackgroundColor,
		app.tick(),
		func() tea.Msg {
			login, _ := app.client.ViewerLogin()
			return viewerMsg(login)
		},
	}
	// Opened on one PR, the list is never shown, so its searches are not run.
	if app.initial.number > 0 {
		commands = append(commands, app.openPullRequest(app.initial, nil, app.initialTab), app.pullRequest.setTab(app, app.initialTab))
	} else {
		commands = append(commands, app.list.fetchAll(app)...)
	}
	return tea.Batch(commands...)
}

// leave goes back from the top of the PR view. Opened from the list, that is
// the list. Opened directly on a PR there is nothing behind it, so q quits
// and esc, which is too easy to hit twice, does nothing.
func (app *App) leave(pressed string) tea.Cmd {
	if app.initial.number == 0 {
		app.screen = screenList
		return nil
	}
	if pressed == "q" {
		return tea.Quit
	}
	return nil
}

// tick animates the spinner while something runs and otherwise only drives
// the slow periodic work.
func (app *App) tick() tea.Cmd {
	interval := time.Second
	if app.busy > 0 || app.anythingLoading() {
		interval = 120 * time.Millisecond
	}
	return tea.Tick(interval, func(now time.Time) tea.Msg { return tickMsg(now) })
}

// poll refetches what has gone stale, unless the pane is in the background or
// a failure's backoff has not run out.
func (app *App) poll() []tea.Cmd {
	if app.unfocused || time.Now().Before(app.network.retryAt) {
		return nil
	}
	commands := app.list.periodic(app)
	if app.screen == screenPullRequest {
		commands = append(commands, app.pullRequest.periodic(app)...)
	}
	return commands
}

func (app *App) anythingLoading() bool {
	for _, section := range app.list.sections() {
		if section.loading {
			return true
		}
	}
	if data := app.store[app.pullRequest.key]; app.screen == screenPullRequest && data != nil {
		return data.loading || data.diffLoading
	}
	return false
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (app *App) spinner() string {
	return spinnerFrames[app.spinnerFrame%len(spinnerFrames)]
}

func (app *App) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if err, isLoad := loadError(message); isLoad {
		app.network.observe(err, app.client.Health())
	}
	switch message := message.(type) {
	case tea.WindowSizeMsg:
		// The layout assumes some room; a real program starts out reporting 0×0.
		app.width, app.height = max(message.Width, 20), max(message.Height, 10)
		app.pullRequest.diff.invalidate()
		return app, nil

	case tea.BackgroundColorMsg:
		var colors *config.ColorTheme
		if app.config.Theme != nil {
			colors = app.config.Theme.Colors
		}
		app.theme = NewTheme(message.IsDark(), colors)
		app.pullRequest.diff.invalidate()
		return app, nil

	case viewerMsg:
		app.viewer = string(message)
		return app, nil

	case tickMsg:
		app.spinnerFrame++
		commands := []tea.Cmd{app.tick()}
		return app, tea.Batch(append(commands, app.poll()...)...)

	case tea.BlurMsg:
		app.unfocused = true
		return app, nil

	case tea.FocusMsg:
		// Catch up at once on whatever went stale while the pane was in the background.
		app.unfocused = false
		return app, tea.Batch(app.poll()...)

	case sectionLoadedMsg:
		return app, app.list.loaded(app, message)

	case previewMsg:
		return app, app.list.previewDue(app, message)

	case pullRequestLoadedMsg:
		return app, app.pullRequestLoaded(message)

	case diffLoadedMsg:
		data := app.data(message.key)
		data.diffLoading = false
		data.diffErr = message.err
		if message.err == nil {
			data.files, data.generated, data.diffHead = message.files, message.generated, message.head
		}
		if message.key == app.pullRequest.key {
			app.pullRequest.diff.setData(app)
		}
		return app, nil

	case jobDetailMsg:
		app.pullRequest.checks.stepsLoaded(message)
		return app, nil

	case jobLogMsg:
		app.pullRequest.checks.logLoaded(app, message)
		return app, nil

	case checksSettledMsg:
		if message.settle == app.pullRequest.checks.settle {
			return app, app.pullRequest.checks.load(app)
		}
		return app, nil

	case actionMsg:
		app.busy = max(app.busy-1, 0)
		if message.err != nil {
			app.notify(firstLine(message.err), true)
			return app, nil
		}
		if message.success != "" {
			app.notify(message.success, false)
		}
		if message.then != nil {
			return app, message.then(app)
		}
		return app, nil

	case customCommandDoneMsg:
		if message.err != nil {
			app.notify(firstLine(message.err), true)
		}
		return app, app.list.refreshCurrent(app)

	case tea.KeyPressMsg:
		return app, app.handleKey(message)

	case tea.PasteMsg:
		if app.overlay != nil {
			return app, app.updateOverlay(message)
		}
		if app.list.searching {
			return app, app.list.updateSearch(app, message)
		}
	}
	return app, nil
}

func (app *App) handleKey(key tea.KeyPressMsg) tea.Cmd {
	if app.overlay != nil {
		return app.updateOverlay(key)
	}
	if app.screen == screenList && app.list.searching {
		return app.list.updateSearch(app, key)
	}
	pressed := key.String()
	if pressed == "ctrl+c" {
		return tea.Quit
	}
	if app.showHelp {
		app.showHelp = false
		return nil
	}
	if app.screen == screenPullRequest {
		return app.pullRequest.handleKey(app, pressed)
	}
	return app.list.handleKey(app, pressed)
}

func (app *App) View() tea.View {
	var lines []string
	if app.screen == screenPullRequest {
		lines = app.pullRequest.view(app, app.height-1)
	} else {
		lines = app.list.view(app, app.height-1)
	}
	lines = block(lines, app.width, app.height-1)
	if chip := app.statusChip(); chip != "" {
		room := app.width - width(chip) - 3
		top := lines[0]
		plain := strings.TrimRight(ansi.Strip(top), " ")
		// The chip takes the place of what sits at the far right (the logo, the
		// PR's title) instead of cutting into it.
		if gap := strings.LastIndex(plain, "   "); width(plain) > room && gap >= 0 {
			plain = strings.TrimRight(plain[:gap], " ")
		}
		if width(plain) <= room {
			top = ansi.Truncate(top, width(plain), "")
		}
		lines[0] = spread(truncate(top, room), chip+" ", app.width)
	}
	if app.showHelp {
		lines = overlayOnto(lines, app.helpBox(), app.width, false)
	}
	if app.overlay != nil {
		if box := app.overlayBox(); box != "" {
			lines = overlayOnto(lines, box, app.width, app.overlay.kind == overlayComposer)
		}
	}
	lines = append(lines, app.footer())

	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.ReportFocus = true
	view.WindowTitle = "gh sift"
	return view
}

func (app *App) notify(text string, failure bool) {
	app.toast = toast{text: text, failure: failure, until: time.Now().Add(6 * time.Second)}
}

func firstLine(err error) string {
	text, _, _ := strings.Cut(strings.TrimSpace(err.Error()), "\n")
	return text
}

// run performs a write in the background, then reports and refreshes.
func (app *App) run(doing string, work func() error, success string, then func(app *App) tea.Cmd) tea.Cmd {
	app.busy++
	app.busyText = doing
	return func() tea.Msg {
		return actionMsg{success: success, err: work(), then: then}
	}
}

func (app *App) data(key pullRequestKey) *pullRequestData {
	data := app.store[key]
	if data == nil {
		data = &pullRequestData{}
		app.store[key] = data
	}
	return data
}

// loadPullRequest fetches the detail unless a recent copy is cached.
func (app *App) loadPullRequest(key pullRequestKey, force bool) tea.Cmd {
	data := app.data(key)
	if data.loading || (!force && data.detail != nil && time.Since(data.detailAt) < 30*time.Second) {
		return nil
	}
	data.loading = true
	return func() tea.Msg {
		detail, err := app.client.PullRequest(key.repository, key.number)
		return pullRequestLoadedMsg{key: key, detail: detail, err: err}
	}
}

func (app *App) loadDiff(key pullRequestKey) tea.Cmd {
	data := app.data(key)
	// The diff only changes when the head moves.
	if data.diffLoading || (data.detail != nil && data.diffHead == data.detail.HeadRefOid && data.diffErr == nil && data.files != nil) {
		return nil
	}
	data.diffLoading = true
	head := ""
	if data.detail != nil {
		head = data.detail.HeadRefOid
	}
	return func() tea.Msg {
		unified, err := app.client.Diff(key.repository, key.number)
		files := diff.Parse(unified)
		// Without the attributes, the built-in guesses still fold lockfiles and the like.
		var generated map[string]bool
		if attributes, attributesErr := app.client.Attributes(key.repository, head); attributesErr == nil && head != "" {
			paths := make([]string, len(files))
			for index, file := range files {
				paths[index] = file.Path()
			}
			generated, _ = diff.GeneratedByAttributes(attributes, paths)
		}
		return diffLoadedMsg{key: key, head: head, files: files, generated: generated, err: err}
	}
}

func (app *App) pullRequestLoaded(message pullRequestLoadedMsg) tea.Cmd {
	data := app.data(message.key)
	data.loading = false
	data.err = message.err
	if message.err != nil {
		if data.detail != nil {
			// A failed refresh must not take a loaded PR off the screen.
			data.err = nil
			app.notify("refresh failed: "+firstLine(message.err), true)
		}
		return nil
	}
	data.detail, data.detailAt = message.detail, time.Now()
	app.list.updateRow(message.detail)

	if app.screen != screenPullRequest {
		return nil
	}
	if message.key != app.pullRequest.key {
		// A stack neighbour, fetched ahead: while reading diffs, have its diff ready too.
		if app.pullRequest.tab == tabFiles && message.key.repository == app.pullRequest.key.repository {
			return app.loadDiff(message.key)
		}
		return nil
	}
	var commands []tea.Cmd
	// Neighbours in the stack are one keypress away, so have them ready.
	if stack := message.detail.Stack; stack != nil {
		for _, delta := range []int{1, -1} {
			if neighbour := stack.Neighbour(message.detail.StackPosition, delta); neighbour != nil {
				commands = append(commands, app.loadPullRequest(pullRequestKey{message.key.repository, neighbour.Number}, false))
			}
		}
	}
	if app.pullRequest.tab == tabFiles || data.files != nil {
		commands = append(commands, app.loadDiff(message.key))
	}
	app.pullRequest.detailChanged(app)
	return tea.Batch(commands...)
}

// openPullRequest enters the PR view. The list row, when there is one,
// fills the header until the detail arrives.
func (app *App) openPullRequest(key pullRequestKey, seed *gh.PullRequestRow, tab pullRequestTab) tea.Cmd {
	previous := app.pullRequest
	app.pullRequest = newPullRequestScreen(key, seed, tab)
	if previous.key.repository == key.repository && app.screen == screenPullRequest {
		// Moving within a stack keeps the reading setup.
		app.pullRequest.diff.layout = previous.diff.layout
		app.pullRequest.diff.hideSidebar = previous.diff.hideSidebar
		app.pullRequest.diff.dimRead = previous.diff.dimRead
		app.pullRequest.hideResolved = previous.hideResolved
		app.pullRequest.checks.flat = previous.checks.flat
		app.pullRequest.diff.returnToList = previous.diff.returnToList
	} else {
		app.pullRequest.diff.returnToList = tab == tabFiles
	}
	app.screen = screenPullRequest
	app.pullRequest.detailChanged(app)
	commands := []tea.Cmd{app.loadPullRequest(key, false)}
	if tab == tabFiles {
		commands = append(commands, app.loadDiff(key))
	}
	return tea.Batch(commands...)
}

// moveInStack opens the PR above (+1) or below (-1) the given one.
func (app *App) moveInStack(repository string, stack *gh.Stack, position, delta int, tab pullRequestTab) tea.Cmd {
	if stack == nil {
		app.notify("this PR is not part of a stack", false)
		return nil
	}
	neighbour := stack.Neighbour(position, delta)
	if neighbour == nil {
		app.notify(map[int]string{1: "top of the stack", -1: "bottom of the stack"}[delta], false)
		return nil
	}
	// Enough of a row to draw the header and to keep moving through the
	// stack before this PR's own detail has arrived.
	seed := &gh.PullRequestRow{
		Repository: repository, Number: neighbour.Number, Title: neighbour.Title, State: neighbour.State,
		IsDraft: neighbour.IsDraft, IsInMergeQueue: neighbour.IsInMergeQueue,
		Stack: stack, StackPosition: neighbour.Position,
	}
	return app.openPullRequest(pullRequestKey{repository, neighbour.Number}, seed, tab)
}

func (app *App) openInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		// Honours $BROWSER and gh's browser setting, like `gh browse`.
		err := browser.New("", os.Stdout, os.Stderr).Browse(url)
		return actionMsg{err: err}
	}
}

func (app *App) copy(text, what string) tea.Cmd {
	app.notify("copied "+what, false)
	// OSC 52 reaches the local clipboard through SSH and tmux.
	return tea.SetClipboard(text)
}

type customCommandDoneMsg struct{ err error }

// runCustomCommand runs a user keybinding's command the way gh-dash does:
// rendered as a Go template, through $SHELL, with the terminal handed over.
func (app *App) runCustomCommand(commandTemplate string, values map[string]any) tea.Cmd {
	if repository, ok := values["RepoName"].(string); ok {
		if path, found := app.config.RepoPath(repository); found {
			values["RepoPath"] = path
		}
	}
	if _, ok := values["RepoPath"]; !ok && app.repositoryPath != "" {
		values["RepoPath"] = app.repositoryPath
	}
	parsed, err := template.New("keybinding").Option("missingkey=error").Parse(commandTemplate)
	if err != nil {
		app.notify("keybinding template: "+firstLine(err), true)
		return nil
	}
	var rendered bytes.Buffer
	if err := parsed.Execute(&rendered, values); err != nil {
		app.notify("keybinding template: "+firstLine(err), true)
		return nil
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	command := exec.Command(shell, "-c", rendered.String())
	return tea.ExecProcess(command, func(err error) tea.Msg {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			err = fmt.Errorf("command exited with status %d", exit.ExitCode())
		}
		return customCommandDoneMsg{err: err}
	})
}
