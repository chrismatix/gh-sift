package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chrismatix/gh-sift/internal/gh"
)

type pullRequestTab int

const (
	tabOverview pullRequestTab = iota
	tabChecks
	tabThreads
	tabFiles
)

type pullRequestScreen struct {
	key pullRequestKey
	// The list row this was opened from; it fills the header until the
	// detail has loaded.
	seed *gh.PullRequestRow
	tab  pullRequestTab

	overviewScroll int
	// Rendered overview, kept until the data or the width changes.
	overviewLines []string
	overviewWidth int

	checks checksView

	threadCursor int
	hideResolved bool
	threadScroll int

	diff diffView
}

func newPullRequestScreen(key pullRequestKey, seed *gh.PullRequestRow, tab pullRequestTab) pullRequestScreen {
	return pullRequestScreen{key: key, seed: seed, tab: tab, hideResolved: true}
}

func (screen *pullRequestScreen) detail(app *App) *gh.PullRequest {
	return app.data(screen.key).detail
}

// row is the header's data: the detail when loaded, else the list row.
func (screen *pullRequestScreen) row(app *App) *gh.PullRequestRow {
	if detail := screen.detail(app); detail != nil {
		return &detail.PullRequestRow
	}
	return screen.seed
}

func (screen *pullRequestScreen) detailChanged(app *App) {
	screen.overviewLines = nil
	detail := screen.detail(app)
	if detail == nil {
		return
	}
	screen.checks.setChecks(detail.Checks)
	screen.threadCursor = clamp(screen.threadCursor, 0, max(len(screen.threads(app))-1, 0))
	screen.diff.setData(app)
}

// threads lists open threads first; resolved ones only when asked for.
func (screen *pullRequestScreen) threads(app *App) []gh.Thread {
	detail := screen.detail(app)
	if detail == nil {
		return nil
	}
	var open, resolved []gh.Thread
	for _, thread := range detail.Threads {
		if thread.IsResolved {
			resolved = append(resolved, thread)
		} else {
			open = append(open, thread)
		}
	}
	if screen.hideResolved {
		return open
	}
	return append(open, resolved...)
}

func (screen *pullRequestScreen) periodic(app *App) []tea.Cmd {
	var commands []tea.Cmd
	data := app.data(screen.key)
	if app.overlay == nil && app.busy == 0 && !data.loading && time.Since(data.detailAt) > 20*time.Second {
		commands = append(commands, app.loadPullRequest(screen.key, true))
	}
	if screen.tab == tabChecks {
		commands = append(commands, screen.checks.load(app))
	}
	return commands
}

func (screen *pullRequestScreen) setTab(app *App, tab pullRequestTab) tea.Cmd {
	screen.tab = tab
	switch tab {
	case tabChecks:
		return screen.checks.load(app)
	case tabFiles:
		return app.loadDiff(screen.key)
	}
	return nil
}

func (screen *pullRequestScreen) handleKey(app *App, pressed string) tea.Cmd {
	detail := screen.detail(app)
	row := screen.row(app)
	// The Checks tab has panes of its own and a search box, so it sees keys first.
	if screen.tab == tabChecks && detail != nil {
		if handled, command := screen.checks.handleKey(app, pressed); handled {
			return command
		}
	}

	// While a diff search is typed, every key is part of the query.
	if screen.tab == tabFiles && screen.diff.typing {
		return screen.diff.handleKey(app, pressed)
	}

	// Keys that mean the same on every tab.
	switch pressed {
	case "?":
		app.showHelp = true
		return nil
	case "tab":
		return screen.setTab(app, (screen.tab+1)%4)
	case "shift+tab":
		return screen.setTab(app, (screen.tab+3)%4)
	case "K", "J":
		if row == nil {
			return nil
		}
		return app.moveInStack(screen.key.repository, row.Stack, row.StackPosition, map[string]int{"K": 1, "J": -1}[pressed], screen.tab)
	case "R":
		app.notify("refreshing", false)
		return app.loadPullRequest(screen.key, true)
	case "m":
		return app.startMerge(screen.key)
	case "a":
		return screen.startReview(app)
	case "o":
		if row != nil {
			return app.openInBrowser(row.URL)
		}
		return nil
	case "y":
		return app.copy(strconv.Itoa(screen.key.number), fmt.Sprintf("#%d", screen.key.number))
	case "Y":
		if row != nil {
			return app.copy(row.URL, row.URL)
		}
		return nil
	}

	if screen.tab == tabFiles {
		return screen.diff.handleKey(app, pressed)
	}

	switch pressed {
	case "esc", "q":
		return app.leave(pressed)
	case "1":
		return screen.setTab(app, tabOverview)
	case "2":
		return screen.setTab(app, tabChecks)
	case "3":
		return screen.setTab(app, tabThreads)
	case "4", "d":
		return screen.setTab(app, tabFiles)
	case "c":
		app.openComposer(fmt.Sprintf("Comment on #%d", screen.key.number), "", func(app *App, body string) tea.Cmd {
			key := app.pullRequest.key
			return app.run("commenting", func() error { return app.client.Comment(key.repository, key.number, body) },
				"comment posted", func(app *App) tea.Cmd { return app.loadPullRequest(key, true) })
		})
		return nil
	}
	if detail == nil {
		return nil
	}

	half := max((app.height-10)/2, 1)
	move := map[string]int{"j": 1, "down": 1, "k": -1, "up": -1, "ctrl+d": half, "ctrl+u": -half, "g": -1 << 30, "home": -1 << 30, "G": 1 << 30, "end": 1 << 30}[pressed]

	switch screen.tab {
	case tabOverview:
		screen.overviewScroll = max(screen.overviewScroll+move, 0)
	case tabThreads:
		threads := screen.threads(app)
		if move != 0 {
			screen.threadCursor = clamp(screen.threadCursor+move, 0, max(len(threads)-1, 0))
			screen.threadScroll = 0
			return nil
		}
		if pressed == "h" {
			screen.hideResolved = !screen.hideResolved
			screen.threadCursor = 0
			return nil
		}
		if screen.threadCursor >= len(threads) {
			return nil
		}
		thread := threads[screen.threadCursor]
		switch pressed {
		case "r":
			return app.startReply(thread)
		case "x":
			return app.toggleResolved(thread)
		case "enter":
			// Outdated threads have no place in the current diff.
			if thread.Line == 0 {
				app.notify("this thread is outdated: its line is no longer in the diff", false)
				return nil
			}
			screen.diff.pendingThread = thread.ID
			return screen.setTab(app, tabFiles)
		case "ctrl+f", "pgdown":
			screen.threadScroll += half
		case "ctrl+b", "pgup":
			screen.threadScroll = max(screen.threadScroll-half, 0)
		}
	}
	return nil
}

func (app *App) startReply(thread gh.Thread) tea.Cmd {
	author := "the thread"
	if len(thread.Comments) > 0 {
		author = "@" + thread.Comments[0].Author
	}
	key := app.pullRequest.key
	app.openComposer(fmt.Sprintf("Reply to %s · %s:%d", author, thread.Path, max(thread.Line, thread.OriginalLine)), "", func(app *App, body string) tea.Cmd {
		return app.run("replying", func() error { return app.client.ReplyToThread(thread.ID, body) },
			"reply posted", func(app *App) tea.Cmd { return app.loadPullRequest(key, true) })
	})
	return nil
}

func (app *App) toggleResolved(thread gh.Thread) tea.Cmd {
	key := app.pullRequest.key
	done := map[bool]string{true: "thread reopened", false: "thread resolved"}[thread.IsResolved]
	return app.run("updating thread", func() error { return app.client.SetThreadResolved(thread.ID, !thread.IsResolved) },
		done, func(app *App) tea.Cmd { return app.loadPullRequest(key, true) })
}

// startReview asks for the verdict, then an optional body. Comments written
// in the diff are waiting in the pending review and go out with it.
func (screen *pullRequestScreen) startReview(app *App) tea.Cmd {
	detail := screen.detail(app)
	if detail == nil {
		return nil
	}
	key := screen.key
	pending := detail.PendingComments()
	items := []menuItem{
		{label: "Comment", hint: "general feedback, no verdict", value: "COMMENT"},
		{label: "Approve", value: "APPROVE"},
		{label: "Request changes", value: "REQUEST_CHANGES"},
	}
	title := "Submit review"
	if pending > 0 {
		title = fmt.Sprintf("Submit review with %s", plural(pending, "pending comment"))
		items = append(items, menuItem{label: "Discard the pending review", hint: "deletes its " + plural(pending, "comment"), value: "DISCARD"})
	}
	reload := func(app *App) tea.Cmd { return tea.Batch(app.loadPullRequest(key, true), app.list.refreshCurrent(app)) }
	app.openMenu(title, items, func(app *App, event string) tea.Cmd {
		if event == "DISCARD" {
			return app.run("discarding", func() error { return app.client.DiscardPendingReview(detail.PendingReviewID) }, "pending review discarded", reload)
		}
		initial := ""
		if event == "APPROVE" {
			initial = app.config.Defaults.PrApproveComment
		}
		name := map[string]string{"COMMENT": "Comment", "APPROVE": "Approve", "REQUEST_CHANGES": "Request changes"}[event]
		// GitHub refuses only a comment review that has neither a summary nor inline comments.
		optional := event != "COMMENT" || pending > 0
		title := name + " · review summary"
		if !optional {
			title += " (required without inline comments)"
		}
		app.openComposer(title, initial, func(app *App, body string) tea.Cmd {
			return app.run("submitting review", func() error { return app.client.SubmitReview(detail.ID, detail.PendingReviewID, event, body) },
				"review submitted: "+name, reload)
		})
		app.overlay.allowEmpty = optional
		return nil
	})
	return nil
}

func (screen *pullRequestScreen) hints(app *App) string {
	common := " · K/J stack · a review · m merge · esc back"
	hints := "j/k scroll · c comment · d diff" + common
	switch screen.tab {
	case tabChecks:
		hints = screen.checks.hints()
	case tabThreads:
		hints = "j/k move · r reply · x resolve · h resolved · enter show in diff" + common
	case tabFiles:
		hints = screen.diff.hints(app)
	}
	// Opened directly on a PR, leaving it means quitting.
	if app.initial.number > 0 && (screen.tab != tabFiles || screen.diff.returnToList) {
		hints = strings.Replace(hints, "esc back", "q quit", 1)
	}
	return hints
}
