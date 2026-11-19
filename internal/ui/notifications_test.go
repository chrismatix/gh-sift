package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chrismatix/gh-sift/internal/config"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

func notificationTestNode(id string, unread bool, reason, subjectType, subjectURL, title string) map[string]any {
	return map[string]any{
		"id": id, "unread": unread, "reason": reason, "updated_at": listTimestamp(3 * time.Hour),
		"subject":    map[string]any{"title": title, "url": subjectURL, "type": subjectType},
		"repository": map[string]any{"full_name": "octo/widgets", "html_url": "https://github.com/octo/widgets"},
	}
}

func notificationTestNodes() []any {
	return []any{
		notificationTestNode("1", true, "review_requested", "PullRequest", "https://api.github.com/repos/octo/widgets/pulls/101", "Add retry to the uploader"),
		notificationTestNode("2", true, "mention", "Issue", "https://api.github.com/repos/octo/widgets/issues/55", "Uploader drops files over 2 GB"),
		notificationTestNode("3", false, "subscribed", "Release", "https://api.github.com/repos/octo/widgets/releases/9", "v2.0.0"),
	}
}

// notificationApp starts on the notifications view with the fixture inbox.
// Writes answer 205, as GitHub does.
func notificationApp(t *testing.T, cfg config.Config) *testApp {
	t.Helper()
	cfg.Defaults.View = config.NotificationsView
	app := newTestApp(t, Options{Config: cfg})
	serve := func(request ghtest.Request) (int, any) {
		if strings.Contains(request.Path, "page=1&") {
			return 200, notificationTestNodes()
		}
		return 200, []any{}
	}
	app.github.REST("GET", "notifications?", serve)
	app.github.REST("GET", "repos/", serve)
	for _, method := range []string{"PATCH", "DELETE", "PUT"} {
		app.github.REST(method, "notifications", ghtest.Reply(205, ""))
	}
	listServe(app, listPullRequestNodes(), listIssueNodes())
	listServeDetail(app)
	app.press("R")
	return app
}

func (harness *testApp) writes() []ghtest.Request {
	var found []ghtest.Request
	for _, request := range harness.github.Requests {
		if request.Method != "GET" && request.Path != "graphql" {
			found = append(found, request)
		}
	}
	return found
}

func TestNotificationsView(t *testing.T) {
	app := notificationApp(t, listTestConfig())
	app.expectScreen("All (3)", "Participating", "Review Requested",
		"octo/widgets #101", "Add retry to the uploader", "review requested",
		"octo/widgets #55", "Uploader drops files over 2 GB", "mention",
		"v2.0.0", "subscribed", glyphUnread, glyphNotification, "1/3")
	if strings.Count(app.screen(), glyphUnread) != 2 {
		t.Errorf("two threads are unread:\n%s", app.screen())
	}
	// Sections filter on the reason, here and not on GitHub.
	app.press("l", "l")
	app.expectScreen("Participating (2)", "Add retry to the uploader", "Uploader drops files over 2 GB")
	if strings.Contains(app.screen(), "v2.0.0") {
		t.Error("a subscribed thread is not participating")
	}

	app.press("?")
	app.expectScreen("Notifications", "mark as done", "unsubscribe", "switch to PRs")
	app.press("esc")

	app.press("s")
	if app.list.kind != config.PRsView {
		t.Fatal("s should switch to PRs")
	}
	app.expectScreen("My Pull Requests")
	app.press("s", "s")
	app.expectScreen("Participating (2)")
	if count := len(app.github.Find("per_page=50&page=1")); count != 16 {
		t.Errorf("each section should be fetched twice (start and R), not %d times in all", count)
	}
}

func TestNotificationsCompactAndRefetch(t *testing.T) {
	cfg := listTestConfig()
	cfg.Theme.Ui.Table.Compact = true
	limit := 60
	cfg.NotificationsSections = []config.NotificationsSectionConfig{{Title: "All", Limit: &limit}}
	app := notificationApp(t, cfg)
	if app.list.notificationSections[0].limit != 60 {
		t.Error("a section's own limit should win over notificationsLimit")
	}
	app.expectScreen("octo/widgets #101 Add retry to the uploader")
	// The periodic refetch reaches notifications like any section.
	app.list.current().loadedAt = time.Now().Add(-time.Hour)
	before := len(app.github.Find("notifications?"))
	app.send(tickMsg(time.Now()))
	if len(app.github.Find("notifications?")) != before+1 {
		t.Error("a stale notifications section should be refetched")
	}
}

func TestNotificationsOpen(t *testing.T) {
	app := notificationApp(t, listTestConfig())

	app.press("enter")
	if app.App.screen != screenPullRequest || app.pullRequest.key != (pullRequestKey{"octo/widgets", 101}) {
		t.Fatal("enter on a PR notification should open sift's PR view")
	}
	app.expectScreen("Add retry to the uploader (fresh)")
	if writes := app.writes(); len(writes) != 1 || writes[0].Method != "PATCH" || writes[0].Path != "notifications/threads/1" {
		t.Errorf("opening should mark the thread read, sent %+v", writes)
	}
	app.press("esc")
	if app.App.screen != screenList || app.list.current().notifications[0].Unread {
		t.Error("back on the list, the thread should be read")
	}

	// A read thread is not marked again; an issue opens in the browser.
	app.press("enter", "esc", "j", "enter")
	if writes := app.writes(); len(writes) != 2 || writes[1].Path != "notifications/threads/2" || app.App.screen != screenList {
		t.Errorf("an issue should open in the browser and be marked read, sent %+v", writes)
	}
	app.press("j", "o")
	if len(app.writes()) != 2 {
		t.Error("o on a read thread only opens it")
	}
	app.press("Y")
	if app.toast.text != "copied https://github.com/octo/widgets/releases" {
		t.Errorf("toast %q", app.toast.text)
	}
}

func TestNotificationsActions(t *testing.T) {
	app := notificationApp(t, listTestConfig())

	app.press("m")
	current := app.list.current()
	if current.notifications[0].Unread || current.cursor != 1 {
		t.Error("m should mark the thread read and move on")
	}

	app.press("u")
	if writes := app.writes(); len(writes) != 2 || writes[1].Method != "DELETE" || writes[1].Path != "notifications/threads/2/subscription" {
		t.Errorf("u should unsubscribe, sent %+v", writes)
	}

	app.press("D")
	if writes := app.writes(); len(writes) != 3 || writes[2].Method != "DELETE" || writes[2].Path != "notifications/threads/2" {
		t.Errorf("D should mark done, sent %+v", writes)
	}
	app.expectScreen("All (2)")
	if strings.Contains(app.screen(), "Uploader drops files") {
		t.Error("a done thread should leave every section")
	}
	app.press("l", "l")
	app.expectScreen("Participating (1)")
	app.press("h", "h")

	// GitHub keeps listing done threads, so sift remembers them.
	contents, _ := os.ReadFile(donePath())
	var done map[string]time.Time
	if json.Unmarshal(contents, &done) != nil || done["2"].IsZero() {
		t.Errorf("done.json = %s", contents)
	}
	app.press("R")
	app.expectScreen("All (2)")
	if again := newListScreen(app.App); again.done["2"].IsZero() {
		t.Error("done marks should survive a restart")
	}

	app.press("M")
	app.expectScreen("Mark every notification on GitHub as read?")
	app.press("y")
	if writes := app.writes(); writes[len(writes)-1].Method != "PUT" || writes[len(writes)-1].Path != "notifications" {
		t.Errorf("M should mark everything read, sent %+v", writes)
	}
	for _, row := range app.list.current().notifications {
		if row.Unread {
			t.Error("every row should be read after M")
		}
	}

	app.send(tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt})
	app.expectScreen("Mark the 2 notifications of this section as done?")
	app.press("y")
	app.expectScreen("All (0)", "marked 2 notifications done")
	if writes := app.writes(); len(writes) != 6 {
		t.Errorf("alt+d should mark both remaining threads done, sent %+v", writes)
	}
	app.send(tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt})
	if app.overlay != nil {
		t.Error("nothing to mark done, nothing to ask")
	}
	app.press("mDu")
	if len(app.writes()) != 6 {
		t.Error("with no row, the thread actions do nothing")
	}
}

func TestNotificationsReadOnly(t *testing.T) {
	app := notificationApp(t, listTestConfig())
	app.client.ReadOnly = true
	app.press("m")
	app.expectScreen("read-only mode")
	app.press("k", "D", "u", "enter", "esc")
	app.send(tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt})
	app.press("y")
	if writes := app.writes(); len(writes) != 0 {
		t.Errorf("read-only mode sent %+v", writes)
	}
	app.expectScreen("All (3)")
	if !app.list.current().notifications[0].Unread {
		t.Error("a refused write should leave the row as it was")
	}
}

func TestNotificationsPagingAndSmartFiltering(t *testing.T) {
	cfg := listTestConfig()
	cfg.NotificationsSections = []config.NotificationsSectionConfig{{Title: "Inbox", Filters: "is:unread"}}
	cfg.Defaults.View = config.NotificationsView
	app := newTestApp(t, Options{Config: cfg, Repository: "octo/widgets"})
	app.github.REST("GET", "repos/octo/widgets/notifications", func(request ghtest.Request) (int, any) {
		var nodes []any
		if strings.Contains(request.Path, "page=1&") {
			for index := range 50 {
				nodes = append(nodes, notificationTestNode(fmt.Sprintf("page1-%d", index), true, "subscribed", "Issue", "", fmt.Sprintf("Thread %d", index)))
			}
		} else if strings.Contains(request.Path, "page=2&") {
			nodes = notificationTestNodes()
		}
		return 200, nodes
	})
	app.press("R")
	app.expectScreen("Inbox (50)", "repo:octo/widgets", "is:unread")
	if paths := app.github.Find("repos/octo/widgets/notifications?per_page=50&page=1&all=false"); len(paths) == 0 {
		t.Error("smart filtering should ask the repository's notifications")
	}
	app.press("G", "z")
	app.expectScreen("Inbox (52)")

	app.press("t")
	if app.list.smartFiltering || len(app.github.Find("notifications?per_page=50&page=1&all=false")) == 0 {
		t.Error("without smart filtering the whole inbox is asked")
	}
}

func TestNotificationsPreview(t *testing.T) {
	cfg := config.Default()
	app := notificationApp(t, cfg)
	app.expectScreen("loading…")
	app.send(previewMsg{key: pullRequestKey{"octo/widgets", 101}, request: app.list.previewRequest})
	app.expectScreen("enter opens the PR and marks it read", "review requested · updated 3h ago", "Retries every upload")
	if len(app.writes()) != 0 {
		t.Error("previewing a PR must not mark its thread read")
	}
	app.press("j")
	app.expectScreen("enter opens it in the browser and marks it read", "octo/widgets · #55")
	app.press("j")
	app.expectScreen("read · subscribed")
}

func TestNotificationsKeybindings(t *testing.T) {
	cfg := listTestConfig()
	cfg.Keybindings.Notifications = []config.Keybinding{
		{Key: "x", Builtin: "markAsDone", Name: "finish"},
		{Key: "b", Builtin: "toggleBookmark"},
		{Key: "ctrl+b", Command: "echo {{.RepoName}} {{.IssueNumber}}"},
	}
	cfg.Keybindings.Prs = []config.Keybinding{{Key: "ctrl+e", Command: "echo {{.PrNumber}} {{.Nope}}"}}
	cfg.Keybindings.Issues = []config.Keybinding{{Key: "ctrl+e", Command: "echo {{.IssueNumber}} {{.Missing}}"}}
	app := notificationApp(t, cfg)

	// On a PR the notification command finds no issue number; the PR command runs on ctrl+e.
	app.press("ctrl+b")
	if !strings.Contains(app.toast.text, "IssueNumber") {
		t.Errorf("toast %q", app.toast.text)
	}
	app.press("ctrl+e")
	if !strings.Contains(app.toast.text, "Nope") {
		t.Errorf("toast %q", app.toast.text)
	}
	app.press("j", "ctrl+e")
	if !strings.Contains(app.toast.text, "Missing") {
		t.Errorf("toast %q", app.toast.text)
	}

	app.press("?")
	app.expectScreen("finish", "Custom", "echo {{.RepoName}}")
	app.press("esc", "b")
	app.expectScreen("toggleBookmark is not supported yet")
	app.press("x")
	if writes := app.writes(); len(writes) != 1 || writes[0].Path != "notifications/threads/2" {
		t.Errorf("x should mark done, sent %+v", writes)
	}

	cfg.Keybindings.Notifications = []config.Keybinding{{Key: "x", Builtin: "nonsense"}}
	if _, err := New(Options{Config: cfg}); err == nil || !strings.Contains(err.Error(), "notification") {
		t.Errorf("an unknown builtin should be refused, got %v", err)
	}
}

func TestNotificationsForgetOldDoneMarks(t *testing.T) {
	app := newTestApp(t, Options{Config: listTestConfig()})
	marks := map[string]time.Time{"old": time.Now().Add(-100 * 24 * time.Hour), "recent": time.Now().Add(-time.Hour)}
	contents, _ := json.Marshal(marks)
	_ = os.MkdirAll(filepath.Dir(donePath()), 0o755)
	_ = os.WriteFile(donePath(), contents, 0o644)
	list := newListScreen(app.App)
	if _, kept := list.done["old"]; kept || list.done["recent"].IsZero() {
		t.Errorf("done = %v", list.done)
	}
}

func TestNotificationGlyphs(t *testing.T) {
	theme := NewTheme(true, nil)
	for subjectType, glyph := range map[string]string{
		"PullRequest": glyphOpenPR, "Issue": glyphOpenIssue, "Discussion": glyphDiscussion, "Release": glyphRelease,
		"Commit": glyphCommit, "CheckSuite": glyphCI, "RepositoryVulnerabilityAlert": glyphNotification,
	} {
		if !strings.Contains(theme.notificationGlyph(subjectType), glyph) {
			t.Errorf("%s should be drawn as %q", subjectType, glyph)
		}
	}
}
