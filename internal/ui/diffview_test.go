package ui

import (
	tea "charm.land/bubbletea/v2"

	"strings"
	"testing"

	"github.com/chrismatix/gh-sift/internal/diff"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
	"github.com/chrismatix/gh-sift/internal/readstate"
)

// diffViewUnified is a PR touching a Go file with two hunks, a lockfile, a
// file .gitattributes may declare generated, a new file, a binary and a pure rename.
const diffViewUnified = `diff --git a/cmd/app/main.go b/cmd/app/main.go
index 1111111..2222222 100644
--- a/cmd/app/main.go
+++ b/cmd/app/main.go
@@ -1,4 +1,4 @@ package main
 package main
-func old() {}
+func renamed() {}
 // keep
 // tail
@@ -20,3 +20,3 @@ func later() {
 	first := 1
+	second := 2
 	return first
-}
diff --git a/go.sum b/go.sum
index 1111111..2222222 100644
--- a/go.sum
+++ b/go.sum
@@ -1,1 +1,1 @@
-example.com/a v1
+example.com/a v2
diff --git a/web/api.ts b/web/api.ts
index 1111111..2222222 100644
--- a/web/api.ts
+++ b/web/api.ts
@@ -1,1 +1,1 @@
-export const version = 1
+export const version = 2
diff --git a/docs/guide.md b/docs/guide.md
new file mode 100644
--- /dev/null
+++ b/docs/guide.md
@@ -0,0 +1,2 @@
+# Guide
+Read	me.
diff --git a/logo.png b/logo.png
index 1111111..2222222 100644
Binary files a/logo.png and b/logo.png differ
diff --git a/old/name.txt b/new/name.txt
similarity index 100%
rename from old/name.txt
rename to new/name.txt
`

func diffViewFile(path, viewed string) map[string]any {
	return map[string]any{"path": path, "additions": 1, "deletions": 1, "viewerViewedState": viewed}
}

func diffViewThread(id, path string, line int, side string, resolved, outdated bool, body string) map[string]any {
	return map[string]any{
		"id": id, "isResolved": resolved, "isOutdated": outdated, "path": path, "line": line,
		"originalLine": line + 1, "diffSide": side,
		"comments": map[string]any{"nodes": []any{map[string]any{"id": id + "c", "author": map[string]any{"login": "reviewer"}, "body": body}}},
	}
}

// diffViewFixture is what the fake GitHub answers for o/r#1.
type diffViewFixture struct {
	unified    string
	attributes string
	files      []any
	threads    []any
}

func diffViewDefaultFixture() diffViewFixture {
	return diffViewFixture{
		unified: diffViewUnified,
		files: []any{
			diffViewFile("cmd/app/main.go", "UNVIEWED"), diffViewFile("go.sum", "UNVIEWED"),
			diffViewFile("web/api.ts", "UNVIEWED"), diffViewFile("docs/guide.md", "UNVIEWED"),
			diffViewFile("logo.png", "UNVIEWED"), diffViewFile("new/name.txt", "UNVIEWED"),
		},
	}
}

func (fixture diffViewFixture) serve(server *ghtest.Server) {
	server.GraphQL("reviewThreads(first: 100)", ghtest.Reply(200, map[string]any{"repository": map[string]any{
		"squashMergeAllowed": true,
		"pullRequest": map[string]any{
			"id": "PR_1", "number": 1, "title": "Rework the app", "url": "https://github.com/o/r/pull/1",
			"state": "OPEN", "headRefOid": "abc123", "author": map[string]any{"login": "octocat"},
			"repository":    map[string]any{"nameWithOwner": "o/r"},
			"reviewThreads": map[string]any{"nodes": fixture.threads},
			"files":         map[string]any{"nodes": fixture.files},
		},
	}}))
	server.REST("GET", "repos/o/r/pulls/1", ghtest.Reply(200, fixture.unified))
	if fixture.attributes != "" {
		server.REST("GET", "repos/o/r/contents/.gitattributes", ghtest.Reply(200, fixture.attributes))
	}
	server.GraphQL("markFileAsViewed", ghtest.Reply(200, map[string]any{}))
	server.GraphQL("ReviewThread(input", ghtest.Reply(200, map[string]any{}))
	server.GraphQL("addPullRequestReviewThread(", ghtest.Reply(200, map[string]any{"addPullRequestReviewThread": map[string]any{"thread": map[string]any{"id": "T_new"}}}))
}

// diffViewOpen opens o/r#1 straight on the diff. The harness starts the app
// before routes can be registered, so that first load fails and R reloads.
func diffViewOpen(t *testing.T, fixture diffViewFixture) *testApp {
	t.Helper()
	app := newTestApp(t, Options{OpenRepository: "o/r", OpenNumber: 1, OpenTab: "files"})
	fixture.serve(app.github)
	app.press("R")
	return app
}

// diffViewCurrent is the row under the cursor.
func diffViewCurrent(app *testApp) diffRow {
	view := &app.pullRequest.diff
	return view.rows[view.cursor]
}

// diffViewGoTo walks the cursor down from the top until it stands on a row
// that passes the test.
func diffViewGoTo(app *testApp, test func(row diffRow) bool) {
	app.t.Helper()
	app.press("g")
	for range len(app.pullRequest.diff.rows) {
		if test(diffViewCurrent(app)) {
			return
		}
		app.press("j")
	}
	app.t.Fatalf("no row passes the test:\n%s", app.screen())
}

func diffViewOnLine(text string) func(row diffRow) bool {
	return func(row diffRow) bool {
		line := row.right
		if line == nil {
			line = row.left
		}
		return row.kind == rowCode && line != nil && line.Text == text
	}
}

func diffViewOnFile(app *testApp, path string) func(row diffRow) bool {
	return func(row diffRow) bool {
		return row.kind == rowFile && app.pullRequest.diff.files[row.file].Path() == path
	}
}

func TestDiffViewShowsTheDiff(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	app.expectScreen(
		"Files  0/6 viewed", "hunks  0/5 read",
		"M  cmd/app/main.go", "@@ -1,4 +1,4 @@ package main", "2     - func old() {}", "2 + func renamed() {}",
		"▸ generated file · 1 hunk folded · z unfolds",
		"A  docs/guide.md", "Binary file, not shown",
		"R  new/name.txt  ← old/name.txt", "No content changes",
		"j/k line · ] [ hunk",
	)
	if row := diffViewCurrent(app); row.kind != rowHunk || row.file != 0 || row.hunk != 0 {
		t.Errorf("the cursor should start on the first hunk, got %+v", row)
	}
	// The cursor's file is framed, the others are not.
	if !strings.Contains(app.screen(), "╰───") {
		t.Errorf("the current file should be framed:\n%s", app.screen())
	}
	if len(app.github.Find("contents/.gitattributes?ref=abc123")) == 0 {
		t.Error("the diff should look up .gitattributes at the head commit")
	}
}

func TestDiffViewLoadStates(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		fixture := diffViewDefaultFixture()
		app := newTestApp(t, Options{OpenRepository: "o/r", OpenNumber: 1, OpenTab: "files"})
		fixture.serve(app.github)
		app.github.REST("GET", "repos/o/r/pulls/1", ghtest.Reply(500, "{}"))
		app.press("R")
		app.expectScreen("HTTP 500")
	})
	t.Run("loading", func(t *testing.T) {
		app := diffViewOpen(t, diffViewDefaultFixture())
		data := app.data(app.pullRequest.key)
		data.files, data.diffErr = nil, nil
		app.expectScreen("Loading the diff…")
	})
	t.Run("no files", func(t *testing.T) {
		fixture := diffViewDefaultFixture()
		fixture.unified, fixture.files = "", nil
		app := diffViewOpen(t, fixture)
		app.expectScreen("This pull request changes no files.")
		app.press("j", "x", "c")
		if app.overlay != nil || len(app.github.Find("mutation")) != 0 {
			t.Error("keys on an empty diff should do nothing")
		}
	})
}

func TestDiffViewMovement(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	view := &app.pullRequest.diff

	app.press("j")
	if !diffViewOnLine("package main")(diffViewCurrent(app)) {
		t.Errorf("j should step onto the first line, got %+v", diffViewCurrent(app))
	}
	app.press("k", "k")
	if view.cursor != 0 {
		t.Errorf("k should reach the file header, cursor = %d", view.cursor)
	}

	app.press("]")
	if row := diffViewCurrent(app); row.kind != rowHunk || row.hunk != 0 {
		t.Errorf("] should land on the first hunk, got %+v", row)
	}
	app.press("]")
	if row := diffViewCurrent(app); row.kind != rowHunk || row.hunk != 1 {
		t.Errorf("] should land on the second hunk, got %+v", row)
	}
	app.press("[")
	if row := diffViewCurrent(app); row.kind != rowHunk || row.hunk != 0 {
		t.Errorf("[ should go back a hunk, got %+v", row)
	}
	app.press("[", "[")
	app.expectScreen("no earlier hunk")

	app.press(".")
	if row := diffViewCurrent(app); row.kind != rowFile || row.file != 1 {
		t.Errorf(". should jump to the next file, got %+v", row)
	}
	app.press(",")
	if row := diffViewCurrent(app); row.kind != rowFile || row.file != 0 {
		t.Errorf(", should jump back a file, got %+v", row)
	}

	// Blank rows between files are skipped.
	diffViewGoTo(app, diffViewOnLine("}"))
	app.press("j")
	if row := diffViewCurrent(app); row.kind != rowFile || row.file != 1 {
		t.Errorf("j should skip the blank row onto the next file, got %+v", row)
	}
	app.press("k")
	if !diffViewOnLine("}")(diffViewCurrent(app)) {
		t.Errorf("k should skip the blank row back, got %+v", diffViewCurrent(app))
	}

	app.press("G")
	if view.cursor != len(view.rows)-1 {
		t.Errorf("G should reach the last row, cursor = %d of %d", view.cursor, len(view.rows))
	}
	app.press("]")
	app.expectScreen("no further hunk")
	app.press("home")
	if view.cursor != 0 {
		t.Errorf("home should reach the top, cursor = %d", view.cursor)
	}

	for _, test := range []struct {
		key   string
		moves int
	}{{"d", 1}, {"u", -1}, {"space", 1}, {"b", -1}, {"ctrl+d", 1}, {"pgup", -1}} {
		before := view.cursor
		app.press(test.key)
		if (view.cursor-before)*test.moves <= 0 {
			t.Errorf("%s should move the cursor %+d, went %d → %d", test.key, test.moves, before, view.cursor)
		}
	}

	app.press("right", "right")
	if view.offset != 16 {
		t.Errorf("right scrolls by 8, offset = %d", view.offset)
	}
	app.press("left", "left", "left")
	if view.offset != 0 {
		t.Errorf("left stops at 0, offset = %d", view.offset)
	}
}

func TestDiffViewScrollsCodeSideways(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	app.press("right")
	app.expectScreen("med() {}")
	if strings.Contains(app.screen(), "func renamed") {
		t.Errorf("the start of the line should have scrolled away:\n%s", app.screen())
	}
}

func TestDiffViewFilePicker(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	app.press("/")
	app.expectScreen("Jump to file", "docs/guide.md  +2 -0")
	app.press("guide", "enter")
	if row := diffViewCurrent(app); row.kind != rowFile || row.file != 3 {
		t.Errorf("picking a file should put the cursor on its header, got %+v", row)
	}
}

func TestDiffViewLayouts(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	view := &app.pullRequest.diff

	app.press("1")
	// Rows are rebuilt for a new layout when the diff is next drawn.
	app.screen()
	if !view.rowsSplit {
		t.Error("1 should show old and new side by side")
	}
	// Side by side, the changed line pairs up with the one it replaced.
	if !diffViewOnOneLine(app.screen(), "func old", "func renamed") {
		t.Errorf("the split layout should pair the lines:\n%s", app.screen())
	}
	app.press("2")
	app.screen()
	if view.rowsSplit {
		t.Error("2 should stack the lines")
	}
	app.press("s")
	app.screen()
	if strings.Contains(app.screen(), "0/6 viewed") || view.sidebarWidth(app.App) != 0 {
		t.Errorf("s should hide the sidebar:\n%s", app.screen())
	}
	// Auto: without the sidebar the pane is wide enough to split.
	app.press("0")
	app.screen()
	if !view.rowsSplit {
		t.Error("a wide pane should split by itself")
	}

	app.send(tea.WindowSizeMsg{Width: 80, Height: 30})
	app.press("s")
	app.expectScreen("cmd/app/main.go")
	if view.sidebarWidth(app.App) != 0 || view.rowsSplit {
		t.Error("a narrow screen should have neither the sidebar nor the split")
	}
}

// diffViewOnOneLine reports whether both texts sit on the same screen line.
func diffViewOnOneLine(screen, first, second string) bool {
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, first) && strings.Contains(line, second) {
			return true
		}
	}
	return false
}

func TestDiffViewReadsHunks(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	key := app.pullRequest.key
	view := &app.pullRequest.diff
	firstHash := view.files[0].Hunks[0].Hash()

	app.press("x")
	if !app.readState.IsRead(key.repository, key.number, "cmd/app/main.go", firstHash) {
		t.Fatal("x should record the hunk as read")
	}
	if row := diffViewCurrent(app); row.kind != rowHunk || row.hunk != 1 {
		t.Errorf("x should move on to the next unread hunk, got %+v", row)
	}
	app.expectScreen("✓ read · @@ -1,4 +1,4 @@ package main · 5 lines", "hunks  1/5 read")
	if len(app.github.Find("markFileAsViewed")) != 0 {
		t.Error("the file is not viewed until every hunk is read")
	}

	// Reading the last hunk marks the whole file viewed on GitHub.
	app.press("x")
	if requests := app.github.Find("{ markFileAsViewed("); len(requests) != 1 || requests[0].Variables["path"] != "cmd/app/main.go" {
		t.Fatalf("reading every hunk should mark the file viewed, sent %+v", requests)
	}
	app.expectScreen("Files  1/6 viewed", "▸ viewed · 2 hunks folded")

	// Opening the file again and reopening a hunk takes the mark back.
	diffViewGoTo(app, diffViewOnFile(app, "cmd/app/main.go"))
	app.press("enter")
	app.expectScreen("✓ read · @@ -1,4 +1,4 @@")
	app.press("j", "enter")
	app.expectScreen("✓ read", "1   1   package main")
	app.press("x")
	if len(app.github.Find("unmarkFileAsViewed")) != 1 {
		t.Error("reopening a hunk should clear the file's viewed mark")
	}
	if app.readState.IsRead(key.repository, key.number, "cmd/app/main.go", firstHash) {
		t.Error("x on a read hunk should reopen it")
	}

	// Off a hunk, x explains itself.
	diffViewGoTo(app, diffViewOnFile(app, "docs/guide.md"))
	app.press("x")
	app.expectScreen("x checks off a hunk or a thread")
}

func TestDiffViewShowsReadHunksDimmed(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	app.press("x", "H")
	app.expectScreen("read hunks shown dimmed", "@@ -1,4 +1,4 @@ package main  ✓ read", "func renamed() {}")
	app.press("H")
	app.expectScreen("read hunks folded", "✓ read · @@ -1,4 +1,4 @@")
}

func TestDiffViewMarksFilesViewed(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	key := app.pullRequest.key
	app.press("v")
	requests := app.github.Find("{ markFileAsViewed(")
	if len(requests) != 1 || requests[0].Variables["path"] != "cmd/app/main.go" || requests[0].Variables["id"] != "PR_1" {
		t.Fatalf("v should mark the file viewed on GitHub, sent %+v", requests)
	}
	app.expectScreen("▸ viewed · 2 hunks folded", "✓ viewed", "Files  1/6 viewed", "hunks  2/5 read")
	for _, hunk := range app.pullRequest.diff.files[0].Hunks {
		if !app.readState.IsRead(key.repository, key.number, "cmd/app/main.go", hunk.Hash()) {
			t.Error("viewing a file should read all its hunks")
		}
	}
	app.press("v")
	if len(app.github.Find("unmarkFileAsViewed")) != 1 {
		t.Error("v again should unmark the file")
	}
	app.expectScreen("func renamed() {}", "Files  0/6 viewed")
}

func TestDiffViewWritesFailInReadOnlyMode(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	app.client.ReadOnly = true
	app.press("v")
	if len(app.github.Find("markFileAsViewed")) != 0 {
		t.Error("read-only mode should not reach GitHub")
	}
	app.expectScreen("read-only mode", "✓ viewed")
}

func TestDiffViewViewedStateFromGitHub(t *testing.T) {
	fixture := diffViewDefaultFixture()
	fixture.files[0] = diffViewFile("cmd/app/main.go", "VIEWED")
	fixture.files[3] = diffViewFile("docs/guide.md", "DISMISSED")
	app := diffViewOpen(t, fixture)
	key := app.pullRequest.key
	app.expectScreen("▸ viewed · 2 hunks folded", "↻ changed since viewed", "Files  1/6 viewed", "hunks  2/5 read")
	// GitHub's mark is remembered per hunk, so it survives the file changing.
	for _, hunk := range app.pullRequest.diff.files[0].Hunks {
		if !app.readState.IsRead(key.repository, key.number, "cmd/app/main.go", hunk.Hash()) {
			t.Error("hunks of a viewed file should be recorded as read")
		}
	}
	// The cursor starts on the first hunk still to read.
	if row := diffViewCurrent(app); row.kind != rowHunk || row.file != 2 {
		t.Errorf("the cursor should skip the viewed file, got %+v", row)
	}
}

func TestDiffViewFoldsGeneratedFiles(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	diffViewGoTo(app, diffViewOnFile(app, "go.sum"))
	app.press("z")
	app.expectScreen("example.com/a v2")
	app.press("z")
	app.expectScreen("▸ generated file · 1 hunk folded")
	app.press("j", "enter")
	app.expectScreen("example.com/a v2")

	for _, test := range []struct{ path, want string }{
		{"go.sum", "generated"}, {"web/app.min.js", "generated"}, {"api/types.generated.ts", "generated"}, {"main.go", ""},
	} {
		if got := map[bool]string{true: "generated"}[isGenerated(test.path)]; got != test.want {
			t.Errorf("isGenerated(%q) = %q, want %q", test.path, got, test.want)
		}
	}
}

func TestDiffViewFollowsGitattributes(t *testing.T) {
	fixture := diffViewDefaultFixture()
	fixture.attributes = "web/*.ts linguist-generated\ngo.sum -linguist-generated\n"
	app := diffViewOpen(t, fixture)
	// git reads the attributes, which can outlast the harness's 100ms under
	// load, so the load is run here to the end.
	data := app.data(app.pullRequest.key)
	data.diffLoading, data.diffHead = false, ""
	app.send(app.loadDiff(app.pullRequest.key)())
	app.expectScreen("example.com/a v2")
	diffViewGoTo(app, diffViewOnFile(app, "web/api.ts"))
	app.press("j")
	if row := diffViewCurrent(app); row.kind != rowFold {
		t.Errorf("a file .gitattributes calls generated should fold, got %+v", row)
	}
}

func TestDiffViewThreads(t *testing.T) {
	fixture := diffViewDefaultFixture()
	fixture.threads = []any{
		diffViewThread("T_open", "cmd/app/main.go", 2, "RIGHT", false, false, "**critical** Why rename this?"),
		diffViewThread("T_left", "cmd/app/main.go", 2, "LEFT", false, false, "The old name was fine"),
		diffViewThread("T_done", "cmd/app/main.go", 22, "RIGHT", true, false, "Settled"),
		diffViewThread("T_gone", "cmd/app/main.go", 0, "RIGHT", false, true, "Long gone"),
	}
	app := diffViewOpen(t, fixture)
	app.expectScreen(
		"○ open", "critical Why rename this?", "r reply · x resolve",
		"The old name was fine",
		"✓ resolved · @reviewer: Settled",
		"○ open · outdated, was line 1 · @reviewer: Long gone",
		"● 3",
	)

	app.press("}")
	row := diffViewCurrent(app)
	if row.kind != rowThread || row.thread.ID != "T_left" {
		t.Fatalf("} should reach the first thread, got %+v", row)
	}
	app.expectScreen("r reply · x resolve · enter fold")
	app.press("}")
	if row := diffViewCurrent(app); row.thread == nil || row.thread.ID != "T_open" {
		t.Errorf("} should reach the next thread, got %+v", row)
	}
	app.press("{")
	if row := diffViewCurrent(app); row.thread == nil || row.thread.ID != "T_left" {
		t.Errorf("{ should go back a thread, got %+v", row)
	}

	app.press("x")
	if requests := app.github.Find("resolveReviewThread"); len(requests) != 1 || requests[0].Variables["id"] != "T_left" {
		t.Errorf("x on a thread should resolve it, sent %+v", requests)
	}

	app.press("r")
	app.expectScreen("Reply to @reviewer")
	app.send(tea.PasteMsg{Content: "Agreed"})
	app.press("ctrl+s")
	if requests := app.github.Find("addPullRequestReviewThreadReply"); len(requests) != 1 || requests[0].Variables["body"] != "Agreed" {
		t.Errorf("r should reply to the thread, sent %+v", requests)
	}

	// A resolved thread opens and closes with enter.
	diffViewGoTo(app, func(row diffRow) bool { return row.kind == rowThread && row.thread.ID == "T_done" })
	app.press("enter")
	app.expectScreen("r reply · x reopen · enter collapse")
	app.press("enter")
	app.expectScreen("✓ resolved · @reviewer: Settled")

	app.press("k", "r")
	app.expectScreen("move onto a thread to reply")
}

func TestDiffViewKeepsThreadsOfReadHunks(t *testing.T) {
	fixture := diffViewDefaultFixture()
	fixture.threads = []any{
		diffViewThread("T_new", "cmd/app/main.go", 2, "RIGHT", false, false, "On the new side"),
		diffViewThread("T_old", "cmd/app/main.go", 2, "LEFT", false, false, "On the old side"),
	}
	app := diffViewOpen(t, fixture)
	app.press("x")
	app.expectScreen("✓ read · @@ -1,4 +1,4 @@", "On the new side", "On the old side")
	if strings.Contains(app.screen(), "func renamed") {
		t.Errorf("the read hunk should be folded:\n%s", app.screen())
	}
}

func TestDiffViewOpensOnAThread(t *testing.T) {
	fixture := diffViewDefaultFixture()
	fixture.threads = []any{diffViewThread("T_open", "docs/guide.md", 2, "RIGHT", false, false, "Typo")}
	app := newTestApp(t, Options{OpenRepository: "o/r", OpenNumber: 1})
	fixture.serve(app.github)
	app.press("R", "3", "enter")
	if app.pullRequest.tab != tabFiles {
		t.Fatal("enter on a thread should open the diff")
	}
	app.expectScreen("Typo")
	if row := diffViewCurrent(app); row.kind != rowThread || row.thread.ID != "T_open" {
		t.Errorf("the cursor should be on the thread, got %+v", row)
	}
	// Opened from the overview, esc goes back to it rather than quitting.
	app.press("esc")
	if app.pullRequest.tab != tabOverview {
		t.Error("esc should return to the overview")
	}
}

func TestDiffViewLeaving(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	view := &app.pullRequest.diff
	if command := view.handleKey(app.App, "q"); command == nil {
		t.Fatal("q should quit a diff opened directly")
	} else if _, quit := command().(tea.QuitMsg); !quit {
		t.Error("q should quit a diff opened directly")
	}
	app.press("j", "V")
	if view.anchor < 0 {
		t.Fatal("V should start a selection")
	}
	app.expectScreen("selecting lines")
	app.press("esc")
	if view.anchor >= 0 || app.pullRequest.tab != tabFiles {
		t.Error("esc should only cancel the selection")
	}
	app.press("V", "V")
	if view.anchor >= 0 {
		t.Error("V again should end the selection")
	}
}

func TestDiffViewComments(t *testing.T) {
	added := func(app *testApp) []ghtest.Request { return app.github.Find("addPullRequestReviewThread(") }

	t.Run("on a line", func(t *testing.T) {
		app := diffViewOpen(t, diffViewDefaultFixture())
		diffViewGoTo(app, diffViewOnLine("func renamed() {}"))
		app.press("c")
		app.expectScreen("Comment on cmd/app/main.go:2")
		app.send(tea.PasteMsg{Content: "Nice"})
		app.press("ctrl+s")
		requests := added(app)
		if len(requests) != 1 {
			t.Fatalf("ctrl+s should add a review comment, sent %d", len(requests))
		}
		want := map[string]any{"id": "PR_1", "path": "cmd/app/main.go", "side": "RIGHT", "line": float64(2), "body": "Nice"}
		for name, value := range want {
			if requests[0].Variables[name] != value {
				t.Errorf("%s = %v, want %v", name, requests[0].Variables[name], value)
			}
		}
		app.expectScreen("added to your pending")
	})

	t.Run("on a deleted line", func(t *testing.T) {
		app := diffViewOpen(t, diffViewDefaultFixture())
		diffViewGoTo(app, diffViewOnLine("func old() {}"))
		app.press("c")
		app.send(tea.PasteMsg{Content: "Why"})
		app.press("ctrl+s")
		if requests := added(app); len(requests) != 1 || requests[0].Variables["side"] != "LEFT" || requests[0].Variables["line"] != float64(2) {
			t.Errorf("a deleted line is commented on the old side, sent %+v", requests)
		}
		app.press("S")
		app.expectScreen("a suggestion replaces new code")
	})

	t.Run("on a selection", func(t *testing.T) {
		app := diffViewOpen(t, diffViewDefaultFixture())
		diffViewGoTo(app, diffViewOnLine("package main"))
		app.press("V", "j", "j", "j")
		app.expectScreen("selecting lines")
		app.press("c")
		app.expectScreen("Comment on cmd/app/main.go:1-3")
		app.send(tea.PasteMsg{Content: "Block"})
		app.press("ctrl+s")
		requests := added(app)
		if len(requests) != 1 || requests[0].Variables["startLine"] != float64(1) || requests[0].Variables["line"] != float64(3) {
			t.Errorf("a selection comments on a range, sent %+v", requests)
		}
		if app.pullRequest.diff.anchor >= 0 {
			t.Error("sending should end the selection")
		}
	})

	t.Run("on a selection ending on a deleted line", func(t *testing.T) {
		app := diffViewOpen(t, diffViewDefaultFixture())
		diffViewGoTo(app, diffViewOnLine("// tail"))
		app.press("V")
		for !diffViewOnLine("}")(diffViewCurrent(app)) {
			app.press("j")
		}
		app.press("c")
		app.expectScreen("Comment on cmd/app/main.go:4-22")
		app.send(tea.PasteMsg{Content: "Range"})
		app.press("ctrl+s")
		requests := added(app)
		if len(requests) != 1 || requests[0].Variables["side"] != "LEFT" || requests[0].Variables["startLine"] != float64(4) {
			t.Errorf("the range should be on the old side, sent %+v", requests)
		}
	})

	t.Run("suggestion keeps tabs", func(t *testing.T) {
		app := diffViewOpen(t, diffViewDefaultFixture())
		diffViewGoTo(app, diffViewOnLine("\tsecond := 2"))
		app.press("S")
		app.expectScreen("Suggest a change to cmd/app/main.go:21", "```suggestion")
		app.press("ctrl+s")
		requests := added(app)
		if len(requests) != 1 || requests[0].Variables["body"] != "```suggestion\n\tsecond := 2\n```" {
			t.Errorf("the suggestion should keep the tab, sent %+v", requests)
		}
	})

	t.Run("off code", func(t *testing.T) {
		app := diffViewOpen(t, diffViewDefaultFixture())
		app.press("c")
		app.expectScreen("move onto a line of code")
		if app.overlay != nil {
			t.Error("no composer should open on a hunk header")
		}
	})
}

func TestDiffViewReportsReadStateThatCannotBeSaved(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	app.readState = readstate.Open("/dev/null/read.json")
	app.press("x")
	app.expectScreen("could not save read state")
	app.press("v")
	app.expectScreen("could not save read state")
}

func TestDiffViewPinsTheFileHeader(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	app.send(tea.WindowSizeMsg{Width: 140, Height: 10})
	diffViewGoTo(app, diffViewOnLine("}"))
	lines := strings.Split(app.screen(), "\n")
	if !strings.Contains(lines[2], "cmd/app/main.go") {
		t.Errorf("the file header should stay on top while its file scrolls:\n%s", app.screen())
	}
	if strings.Contains(app.screen(), "@@ -1,4 +1,4 @@") {
		t.Errorf("the first hunk should have scrolled away:\n%s", app.screen())
	}
}

func TestDiffViewVeryNarrowSplit(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	app.send(tea.WindowSizeMsg{Width: 16, Height: 20})
	app.press("1", "j")
	if !app.pullRequest.diff.rowsSplit {
		t.Fatal("1 should split even when narrow")
	}
	// No room for code: only the gutters are left.
	if strings.Contains(app.screen(), "package") {
		t.Errorf("there is no room for code at this width:\n%s", app.screen())
	}
}

func TestDiffViewHugeFileIsNotColoured(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("diff --git a/big.go b/big.go\nnew file mode 100644\n--- /dev/null\n+++ b/big.go\n@@ -0,0 +1,8001 @@\n")
	for range 8001 {
		builder.WriteString("+func f() {}\n")
	}
	fixture := diffViewDefaultFixture()
	fixture.unified = builder.String()
	fixture.files = []any{diffViewFile("big.go", "UNVIEWED")}
	app := diffViewOpen(t, fixture)
	app.expectScreen("1 + func f() {}")
	styling := app.pullRequest.diff.styling[0]
	if styling == nil || len(styling.tokens) != 0 {
		t.Error("a file this large should be drawn without syntax colours")
	}
}

func TestDiffSearchFindsTextInEveryFileAndUnfoldsIt(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	app.press("2", "g")
	// Typed keys belong to the query, not to their usual commands.
	app.press("ctrl+f", "v", "e", "r", "s", "i", "o", "n", "a", "backspace", "enter")
	view := &app.pullRequest.diff
	if view.query != "version" || app.overlay != nil {
		t.Fatalf("query = %q, overlay %v", view.query, app.overlay)
	}
	if row := diffViewCurrent(app); row.left == nil || row.left.Text != "export const version = 1" {
		t.Fatalf("enter should land on the first match, got %+v", row)
	}
	app.press("n")
	if row := diffViewCurrent(app); row.right == nil || row.right.Text != "export const version = 2" {
		t.Errorf("n should move to the next match, got %+v", row)
	}
	app.press("n")
	app.expectScreen("no further match for version")
	app.press("N")
	if row := diffViewCurrent(app); row.left == nil || row.left.Text != "export const version = 1" {
		t.Errorf("N should move back, got %+v", row)
	}

	// go.sum is folded as generated; a match inside opens it.
	app.press("esc", "g", "ctrl+f", "space", "v", "2", "enter")
	if row := diffViewCurrent(app); row.right == nil || row.right.Text != "example.com/a v2" {
		t.Fatalf("the match in the folded file should be revealed, got %+v", row)
	}
	app.expectScreen("search  v2")
	app.press("esc")
	if view.query != "" {
		t.Error("esc should clear the search")
	}
}

func TestDiffSearchHighlightsTheMatch(t *testing.T) {
	view := &diffView{query: "Ver"}
	if spans := view.matchSpans("version, VERSION"); len(spans) != 2 || spans[1] != (diff.Span{Start: 9, End: 12}) {
		t.Errorf("spans = %v", spans)
	}
	if spans := view.matchSpans("İ version"); spans != nil {
		t.Errorf("text whose lowercase changes length is left unmarked: %v", spans)
	}
}
