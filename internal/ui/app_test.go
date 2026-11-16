package ui

import (
	"errors"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chrismatix/gh-sift/internal/config"
	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

func TestAppOpenedOnAPullRequest(t *testing.T) {
	app := newTestApp(t, Options{OpenRepository: "octo/widgets", OpenNumber: 101})
	if len(app.github.Find("search(")) != 0 {
		t.Error("opened on a PR, the list should not search")
	}
	if app.App.screen != screenPullRequest || len(app.github.Find("pullRequest(number:")) != 1 {
		t.Fatal("the PR should open and load at once")
	}
	listServeDetail(app)
	app.press("R")
	app.expectScreen("Add retry to the uploader (fresh)")
	app.press("esc")
	if app.App.screen != screenPullRequest {
		t.Error("esc must not leave a PR there is nothing behind")
	}
	if !listQuitting(app, tea.KeyPressMsg{Code: 'q', Text: "q"}) {
		t.Error("q should quit when the PR was opened directly")
	}
}

func TestAppOpensTheTabNamedOnTheCommandLine(t *testing.T) {
	for tab, want := range map[string]pullRequestTab{"files": tabFiles, "changes": tabFiles, "checks": tabChecks, "": tabOverview} {
		app := newTestApp(t, Options{OpenRepository: "octo/widgets", OpenNumber: 101, OpenTab: tab})
		if app.pullRequest.tab != want {
			t.Errorf("tab %q opened %v, want %v", tab, app.pullRequest.tab, want)
		}
	}
}

func TestAppRejectsAnUnknownBuiltin(t *testing.T) {
	cfg := listTestConfig()
	cfg.Keybindings.Universal = []config.Keybinding{{Key: "z", Builtin: "teleport"}}
	if _, err := New(Options{Config: cfg}); err == nil || !strings.Contains(err.Error(), "teleport") {
		t.Errorf("err = %v", err)
	}
}

func TestAppPrefetchesStackNeighbours(t *testing.T) {
	app := listApp(t, Options{})
	app.press("enter")
	for _, number := range []int{100, 102} {
		if app.data(pullRequestKey{"octo/widgets", number}).detail == nil {
			t.Errorf("#%d sits next to #101 in the stack and should be fetched ahead", number)
		}
	}
	if app.list.current().pullRequests[0].Title != "Add retry to the uploader (fresh)" {
		t.Error("the list row should take the fresh detail")
	}

	// Reading diffs, a neighbour's diff is fetched ahead too.
	app.press("tab", "tab", "tab")
	if app.pullRequest.tab != tabFiles {
		t.Fatalf("tab = %v", app.pullRequest.tab)
	}
	app.send(pullRequestLoadedMsg{key: pullRequestKey{"octo/widgets", 102}, detail: &gh.PullRequest{PullRequestRow: gh.PullRequestRow{Repository: "octo/widgets", Number: 102}, HeadRefOid: "sha102"}})
	if len(app.github.Find("repos/octo/widgets/pulls/102")) == 0 {
		t.Error("the neighbour's diff should be fetched")
	}
	app.press("R")
	if app.data(app.pullRequest.key).diffLoading {
		t.Error("a refresh on an unchanged head should not refetch the diff")
	}
}

func TestAppKeepsTheReadingSetupWithinAStack(t *testing.T) {
	app := listApp(t, Options{})
	app.press("enter")
	app.pullRequest.diff.layout = layoutSplit
	app.pullRequest.hideResolved = false
	app.press("K")
	if app.pullRequest.key.number != 102 {
		t.Fatalf("K should open #102, not #%d", app.pullRequest.key.number)
	}
	if app.pullRequest.diff.layout != layoutSplit || app.pullRequest.hideResolved {
		t.Error("moving within a stack should keep the layout and thread filter")
	}
}

func TestAppPullRequestLoadFailures(t *testing.T) {
	app := listApp(t, Options{})
	key := pullRequestKey{"octo/widgets", 101}
	app.send(pullRequestLoadedMsg{key: key, err: errors.New("HTTP 502\nupstream")})
	if app.data(key).err == nil {
		t.Error("a first load that fails should keep its error")
	}
	app.data(key).detail = &gh.PullRequest{}
	app.send(pullRequestLoadedMsg{key: key, err: errors.New("HTTP 502\nupstream")})
	if app.data(key).err != nil {
		t.Error("a failed refresh must not take the loaded PR away")
	}
	app.expectScreen("refresh failed: HTTP 502")
}

func TestAppDiffLoaded(t *testing.T) {
	app := listApp(t, Options{})
	key := pullRequestKey{"octo/widgets", 101}
	app.send(diffLoadedMsg{key: key, head: "sha1", files: nil, err: errors.New("too large")})
	if data := app.data(key); data.diffErr == nil || data.diffHead != "" {
		t.Error("a failed diff should only record its error")
	}
	app.send(diffLoadedMsg{key: key, head: "sha2", generated: map[string]bool{"go.sum": true}})
	if data := app.data(key); data.diffErr != nil || data.diffHead != "sha2" || !data.generated["go.sum"] {
		t.Errorf("diff data = %+v", data)
	}
}

func TestAppActionResults(t *testing.T) {
	app := listApp(t, Options{})
	app.busy = 1
	app.send(actionMsg{err: errors.New("merge conflict\nin up.go")})
	if app.busy != 0 || !app.toast.failure {
		t.Errorf("busy %d, toast %+v", app.busy, app.toast)
	}
	app.expectScreen("merge conflict")
	if strings.Contains(app.screen(), "in up.go") {
		t.Error("only the first line of an error belongs in the footer")
	}

	followed := false
	app.send(actionMsg{success: "merged #101", then: func(*App) tea.Cmd { followed = true; return nil }})
	if app.busy != 0 || !followed {
		t.Error("a success should finish the work and run its follow-up")
	}
	app.expectScreen("merged #101")

	app.send(actionMsg{})
	if app.busy != 0 {
		t.Error("busy never goes below zero")
	}
}

func TestAppFooterAndSpinner(t *testing.T) {
	app := listApp(t, Options{Repository: "octo/widgets"})
	app.expectScreen("octo/widgets • @octocat")
	frame := app.spinnerFrame
	app.send(tickMsg(time.Now()))
	if app.spinnerFrame != frame+1 {
		t.Error("a tick should advance the spinner")
	}
	app.run(app.App.run("merging", func() error { return nil }, "", nil))
	app.busy = 1
	app.send(tickMsg(time.Now()))
	app.expectScreen(app.spinner() + " merging")

	app.list.current().loading = true
	if !app.anythingLoading() {
		t.Error("a loading section counts as loading")
	}
	app.list.current().loading = false
	app.press("enter")
	app.data(app.pullRequest.key).loading = true
	if !app.anythingLoading() {
		t.Error("the open PR loading counts as loading")
	}
}

func TestAppThemeFollowsTheTerminal(t *testing.T) {
	app := listApp(t, Options{})
	dark := app.theme
	app.send(tea.BackgroundColorMsg{Color: color.White})
	if app.theme == dark {
		t.Error("a light terminal should switch the theme")
	}
	app.config.Theme = nil
	app.send(tea.BackgroundColorMsg{Color: color.Black})
	if app.theme != dark {
		t.Error("a dark terminal should switch it back")
	}
}

func TestAppChecksMessagesReachTheChecksTab(t *testing.T) {
	app := listApp(t, Options{})
	app.press("enter", "2")
	app.send(jobDetailMsg{jobID: 1})
	app.send(jobLogMsg{jobID: 1})
	app.send(checksSettledMsg{settle: app.pullRequest.checks.settle + 1})
	app.send(checksSettledMsg{settle: app.pullRequest.checks.settle})
	if app.pullRequest.tab != tabChecks {
		t.Error("checks messages should leave the tab alone")
	}
	app.expectScreen("·  ")
}

func TestAppKeysAboveTheScreens(t *testing.T) {
	app := listApp(t, Options{})
	if !listQuitting(app, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}) {
		t.Error("ctrl+c should always quit")
	}
	app.press("?")
	app.expectScreen("Navigation and app", "Pull requests", "open the PR")
	app.press("j")
	if app.showHelp || app.list.current().cursor != 0 {
		t.Error("the key that closes help should do nothing else")
	}
	app.send(tea.PasteMsg{Content: "ignored"})
}

func TestAppHelpPerScreen(t *testing.T) {
	cfg := listTestConfig()
	cfg.Keybindings.Prs = []config.Keybinding{{Key: "ctrl+b", Command: "git switch {{.HeadRefName}}", Name: "switch branch"}}
	cfg.Keybindings.Issues = []config.Keybinding{{Key: "ctrl+b", Command: "echo {{.IssueNumber}}"}}
	cfg.Keybindings.Universal = []config.Keybinding{{Key: "ctrl+e", Command: "nvim ."}}
	app := listApp(t, Options{Config: cfg})
	tests := []struct {
		keys []string
		want []string
	}{
		{[]string{"?"}, []string{"Custom", "switch branch", "nvim ."}},
		{[]string{"s", "?"}, []string{"Issues", "switch to PRs", "echo {{.IssueNumber}}"}},
		{[]string{"s", "enter", "?"}, []string{"Pull request", "Overview, Checks, Threads, Files"}},
		{[]string{"2", "?"}, []string{"Checks", "rerun failed runs"}},
		{[]string{"4", "?"}, []string{"Diff", "next / previous hunk"}},
	}
	for _, test := range tests {
		app.press(test.keys...)
		app.expectScreen(test.want...)
		app.press("esc")
	}
}

func TestAppFooterOnThePullRequest(t *testing.T) {
	app := listApp(t, Options{})
	app.press("enter")
	app.expectScreen(app.pullRequest.hints(app.App))
	app.press("2")
	if !strings.Contains(app.screen(), app.pullRequest.hints(app.App)) {
		t.Error("the checks footer should keep the hints")
	}
}

// listExecProbe runs the app in a real program, which is what runs
// tea.ExecProcess, and reports every message it handled. Without a
// terminal the program reports a 0x0 window, so sizes are kept from the test.
type listExecProbe struct {
	app     *App
	handled chan tea.Msg
}

func (probe listExecProbe) Init() tea.Cmd { return nil }

func (probe listExecProbe) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if _, resize := message.(tea.WindowSizeMsg); resize {
		return probe, nil
	}
	_, command := probe.app.Update(message)
	probe.handled <- message
	return probe, command
}

func (probe listExecProbe) View() tea.View { return probe.app.View() }

func TestAppRunsCustomCommandsThroughTheShell(t *testing.T) {
	output := filepath.Join(t.TempDir(), "output")
	cfg := listTestConfig()
	cfg.RepoPaths = map[string]string{"octo/widgets": "/src/widgets"}
	cfg.Keybindings.Prs = []config.Keybinding{{Key: "ctrl+b", Command: "echo {{.RepoName}} {{.PrNumber}} {{.HeadRefName}} {{.RepoPath}} >> " + output}}
	cfg.Keybindings.Universal = []config.Keybinding{{Key: "U", Command: "echo {{.RepoPath}} >> " + output + "; exit 3"}}
	app := listApp(t, Options{Config: cfg, RepositoryPath: "/src/here"})
	t.Setenv("SHELL", "")

	handled := make(chan tea.Msg, 100)
	program := tea.NewProgram(listExecProbe{app.App, handled},
		tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutRenderer(), tea.WithoutSignalHandler())
	finished := make(chan error, 1)
	go func() { _, err := program.Run(); finished <- err }()
	var results []customCommandDoneMsg
	wait := func(until func(message tea.Msg) bool) {
		for {
			select {
			case message := <-handled:
				if done, ok := message.(customCommandDoneMsg); ok {
					results = append(results, done)
				}
				if until(message) {
					return
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the program never got there")
			}
		}
	}
	commandDone := func(message tea.Msg) bool { _, ok := message.(customCommandDoneMsg); return ok }
	program.Send(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	wait(commandDone)
	program.Send(tea.KeyPressMsg{Code: 'U', Text: "U"})
	wait(commandDone)
	program.Quit()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}

	contents, _ := os.ReadFile(output)
	want := "octo/widgets 101 feature-101 /src/widgets\n/src/here\n"
	if string(contents) != want {
		t.Errorf("the commands wrote %q, want %q", contents, want)
	}
	if results[0].err != nil || results[1].err == nil || results[1].err.Error() != "command exited with status 3" {
		t.Errorf("results = %+v", results)
	}
	if !strings.Contains(app.screen(), "command exited with status 3") && app.toast.text != "command exited with status 3" {
		t.Errorf("a failing command should say so, toast %q", app.toast.text)
	}
}

func TestAppCustomCommandTemplateErrors(t *testing.T) {
	cfg := listTestConfig()
	cfg.Keybindings.Prs = []config.Keybinding{{Key: "ctrl+b", Command: "echo {{.Nope}}"}}
	cfg.Keybindings.Issues = []config.Keybinding{{Key: "ctrl+b", Command: "echo {{.IssueNumber}} {{.PrNumber}}"}}
	cfg.Keybindings.Universal = []config.Keybinding{{Key: "U", Command: "echo {{"}}
	app := listApp(t, Options{Config: cfg})
	app.press("ctrl+b")
	app.expectScreen("keybinding template:", "Nope")
	app.press("U")
	app.expectScreen("keybinding template:", "unclosed action")
	// An issue has no PR number: the issue binding is the one that ran.
	app.press("s", "ctrl+b")
	app.expectScreen("keybinding:1:24")

	searches := len(app.github.Find("search("))
	app.send(customCommandDoneMsg{err: errors.New("command exited with status 1")})
	app.expectScreen("command exited with status 1")
	if len(app.github.Find("search(")) != searches+1 {
		t.Error("a finished command should refresh the section")
	}
}

func TestAppTickRefreshesTheOpenPullRequest(t *testing.T) {
	app := listApp(t, Options{})
	app.press("enter")
	loads := len(app.github.Find("pullRequest(number:"))
	app.data(app.pullRequest.key).detailAt = time.Now().Add(-time.Minute)
	app.send(tickMsg(time.Now()))
	if len(app.github.Find("pullRequest(number:")) <= loads {
		t.Error("a stale open PR should be refetched on the tick")
	}
	app.github.GraphQL("pullRequest(number:", ghtest.Reply(200, map[string]any{"repository": map[string]any{"pullRequest": nil}}))
	app.press("R")
	app.expectScreen("refresh failed: octo/widgets#101 not found")
}
