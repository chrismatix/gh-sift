package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

const prRepository = "o/r"

// prDetail is one PR in the shape of gh's pullRequestQuery, open and with
// nothing attached yet. Tests add threads, checks, a stack and so on.
func prDetail(number int) map[string]any {
	return map[string]any{
		"id": fmt.Sprintf("PR_%d", number), "number": number, "title": fmt.Sprintf("Change number %d", number),
		"body": "The **description** of the change.", "url": fmt.Sprintf("https://github.com/o/r/pull/%d", number),
		"state": "OPEN", "additions": 12, "deletions": 3, "changedFiles": 2,
		"headRefName": "feature", "baseRefName": "main", "headRefOid": "abc123",
		"updatedAt": time.Now().Add(-time.Hour), "createdAt": time.Now().Add(-48 * time.Hour),
		"author": map[string]any{"login": "alice"}, "repository": map[string]any{"nameWithOwner": prRepository},
		"mergeStateStatus": "CLEAN", "reviewDecision": "REVIEW_REQUIRED",
	}
}

// prServe answers the PR detail query for the given PRs, looked up by
// number on every request, so a test can change a PR between refreshes.
func prServe(server *ghtest.Server, details ...map[string]any) {
	server.GraphQL("pendingReviews:", func(request ghtest.Request) (int, any) {
		number := int(request.Variables["number"].(float64))
		repository := map[string]any{"squashMergeAllowed": true, "mergeCommitAllowed": true, "rebaseMergeAllowed": true, "pullRequest": nil}
		for _, detail := range details {
			if detail["number"] == number {
				repository["pullRequest"] = detail
			}
		}
		return 200, map[string]any{"repository": repository}
	})
}

// prTestApp opens the first PR directly, as `gh sift o/r#N` does.
func prTestApp(t *testing.T, details ...map[string]any) *testApp {
	t.Helper()
	app := newTestApp(t, Options{OpenRepository: prRepository, OpenNumber: details[0]["number"].(int)})
	prServe(app.github, details...)
	app.run(app.loadPullRequest(app.pullRequest.key, true))
	return app
}

func prNodes(nodes ...any) map[string]any { return map[string]any{"nodes": nodes} }

func prThread(id, path string, line int, comments ...any) map[string]any {
	return map[string]any{
		"id": id, "path": path, "line": line, "originalLine": line, "diffSide": "RIGHT",
		"hunk":     prNodes(map[string]any{"diffHunk": "@@ -1,3 +1,3 @@\n context\n-old\n+new"}),
		"comments": prNodes(comments...),
	}
}

func prComment(author, body string) map[string]any {
	return map[string]any{"id": "C_" + author, "author": map[string]any{"login": author}, "body": body, "createdAt": time.Now().Add(-time.Hour), "state": "SUBMITTED"}
}

// prAnswer registers an empty success for a mutation.
func prAnswer(server *ghtest.Server, mutation string) {
	server.GraphQL(mutation+"(", ghtest.Reply(200, map[string]any{mutation: map[string]any{}}))
}

// prExpectToast checks the footer message, which the footer may truncate.
func prExpectToast(t *testing.T, app *testApp, text string) {
	t.Helper()
	if !strings.Contains(app.toast.text, text) {
		t.Fatalf("toast = %q, want %q", app.toast.text, text)
	}
}

func prVariables(t *testing.T, server *ghtest.Server, mutation string) map[string]any {
	t.Helper()
	found := server.Find(mutation + "(")
	if len(found) != 1 {
		t.Fatalf("%s sent %d times, want once", mutation, len(found))
	}
	return found[0].Variables
}

func TestPrViewShowsLoadErrorThenTheOverview(t *testing.T) {
	app := newTestApp(t, Options{OpenRepository: prRepository, OpenNumber: 7})
	// Nothing answers the detail query yet, so the header has only the key.
	app.expectScreen("o/r #7", "o/r#7 not found")

	app.data(app.pullRequest.key).err = nil
	app.expectScreen("Loading the pull request…")

	detail := prDetail(7)
	detail["labels"] = prNodes(map[string]any{"name": "bug", "color": "ff0000"})
	prServe(app.github, detail)
	app.press("R")
	app.expectScreen(
		"o/r #7", "Change number 7", "Open", "@alice wants to merge feature → main", "+12 -3", "updated 1h ago",
		"description of the change", "No conversation yet.",
		"Review", "review required", "Checks", "none", "Merge", "ready to merge",
		"Overview", "Checks", "Threads", "Files 2",
		"j/k scroll · c comment · d diff · K/J stack · a review · m merge · q quit",
	)
}

func TestPrViewRefreshFailureKeepsTheLoadedPullRequest(t *testing.T) {
	app := prTestApp(t, prDetail(7))
	app.github.GraphQL("pendingReviews:", ghtest.Reply(502, "bad gateway"))
	app.press("R")
	app.expectScreen("Change number 7", "refresh failed")
}

func TestPrViewTimelineShowsEveryKindOfEvent(t *testing.T) {
	detail := prDetail(7)
	detail["body"] = ""
	at := time.Now().Add(-2 * time.Hour)
	actor := map[string]any{"login": "bob"}
	detail["timelineItems"] = prNodes(
		map[string]any{"__typename": "PullRequestCommit", "commit": map[string]any{"abbreviatedOid": "a1b2c3d", "messageHeadline": "first commit", "committedDate": at}},
		map[string]any{"__typename": "PullRequestCommit", "commit": map[string]any{"abbreviatedOid": "e4f5a6b", "messageHeadline": "second commit", "committedDate": at}},
		map[string]any{"__typename": "HeadRefForcePushedEvent", "actor": actor, "createdAt": at},
		map[string]any{"__typename": "ReviewRequestedEvent", "actor": actor, "createdAt": at, "requestedReviewer": map[string]any{"login": "carol"}},
		map[string]any{"__typename": "IssueComment", "author": actor, "body": "a plain comment", "createdAt": at},
		map[string]any{"__typename": "PullRequestReview", "author": map[string]any{"login": "carol"}, "state": "APPROVED", "body": "ship it", "createdAt": at},
		map[string]any{"__typename": "PullRequestReview", "author": map[string]any{"login": "dave"}, "state": "CHANGES_REQUESTED", "body": "not yet", "createdAt": at},
		map[string]any{"__typename": "PullRequestReview", "author": map[string]any{"login": "erin"}, "state": "COMMENTED", "body": "some thoughts", "createdAt": at},
		map[string]any{"__typename": "ClosedEvent", "actor": actor, "createdAt": at},
		map[string]any{"__typename": "ReopenedEvent", "actor": actor, "createdAt": at},
		map[string]any{"__typename": "MergedEvent", "actor": actor, "createdAt": at},
	)
	app := prTestApp(t, detail)
	app.expectScreen(
		"No description.",
		"◦ a1b2c3d  first commit", "◦ e4f5a6b  second commit",
		"↻ @bob force-pushed", "@bob requested review from @carol",
		"● @bob commented", "│ a plain comment",
		"✓ @carol approved", "│ ship it", "✗ @dave requested changes", "│ not yet", "● @erin reviewed",
		"@bob closed this", "@bob reopened this", "@bob merged",
	)
	// Consecutive commits stay tight; other events are set apart.
	screen := app.screen()
	if strings.Index(screen, "second commit")-strings.Index(screen, "first commit") > 300 {
		t.Errorf("commits should sit on consecutive lines:\n%s", screen)
	}
}

func TestPrViewOverviewScrollsAndFoldsTheRailWhenNarrow(t *testing.T) {
	detail := prDetail(7)
	var body []string
	for index := range 80 {
		body = append(body, fmt.Sprintf("paragraph %d", index), "")
	}
	detail["body"] = strings.Join(body, "\n")
	app := prTestApp(t, detail)
	app.expectScreen("paragraph 0", "ready to merge")

	app.press("jjj")
	if app.pullRequest.overviewScroll != 3 {
		t.Errorf("scroll = %d after jjj, want 3", app.pullRequest.overviewScroll)
	}
	app.press("k", "G")
	app.expectScreen("paragraph 79")
	if strings.Contains(app.screen(), "paragraph 0\n") {
		t.Error("G should scroll to the end")
	}
	app.press("g")
	if app.pullRequest.overviewScroll != 0 {
		t.Errorf("g should scroll to the top, scroll = %d", app.pullRequest.overviewScroll)
	}

	// Below 110 columns the status moves above the description.
	app.send(tea.WindowSizeMsg{Width: 100, Height: 40})
	screen := app.screen()
	if !strings.Contains(screen, "ready to merge") || strings.Index(screen, "ready to merge") > strings.Index(screen, "paragraph 0") {
		t.Errorf("a narrow overview should put the status first:\n%s", screen)
	}
}

func TestPrViewTabsCycleAndShowTheirHints(t *testing.T) {
	app := prTestApp(t, prDetail(7))
	app.github.REST("GET", "repos/o/r/pulls/7", ghtest.Reply(200, "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"))

	tests := []struct {
		keys string
		tab  pullRequestTab
		hint string
	}{
		{"tab", tabChecks, "No checks reported."},
		{"tab", tabThreads, "r reply · x resolve"},
		{"shift+tab", tabChecks, "ctrl+r rerun failed"},
		{"3", tabThreads, "No review threads."},
		{"1", tabOverview, "c comment · d diff"},
		{"shift+tab", tabFiles, "main.go"},
		{"tab", tabOverview, "description of the change"},
		{"2", tabChecks, "0 checks"},
		{"d", tabFiles, "#7 Change number 7"},
	}
	for _, test := range tests {
		app.press(test.keys)
		if app.pullRequest.tab != test.tab {
			t.Fatalf("after %s the tab is %d, want %d", test.keys, app.pullRequest.tab, test.tab)
		}
		app.expectScreen(test.hint)
	}
	if len(app.github.Find("repos/o/r/pulls/7")) != 1 {
		t.Error("the diff should be fetched once, when the Files tab opens")
	}
}

func TestPrViewGlobalKeys(t *testing.T) {
	app := prTestApp(t, prDetail(7))

	app.press("y")
	app.expectScreen("copied #7")
	app.press("Y")
	app.expectScreen("copied https://github.com/o/r/pull/7")
	app.press("o")
	if app.toast.failure {
		t.Errorf("opening the browser failed: %s", app.toast.text)
	}

	app.press("?")
	if !app.showHelp {
		t.Error("? should show the help")
	}
	app.press("j")
	if app.showHelp || app.pullRequest.overviewScroll != 0 {
		t.Error("the first key after the help only closes it")
	}

	requests := len(app.github.Find("pendingReviews:"))
	app.press("R")
	app.expectScreen("refreshing")
	if len(app.github.Find("pendingReviews:")) != requests+1 {
		t.Error("R should fetch the PR again")
	}

	// Opened directly on a PR, q quits and esc does nothing.
	if command := app.pullRequest.handleKey(app.App, "q"); command == nil {
		t.Error("q should quit")
	} else if _, quits := command().(tea.QuitMsg); !quits {
		t.Error("q should quit")
	}
	if command := app.pullRequest.handleKey(app.App, "esc"); command != nil {
		t.Error("esc should not quit")
	}
}

func TestPrViewKeysBeforeTheDetailLoads(t *testing.T) {
	app := newTestApp(t, Options{OpenRepository: prRepository, OpenNumber: 7})
	app.press("K", "o", "Y", "j", "a")
	if app.overlay != nil || app.pullRequest.overviewScroll != 0 {
		t.Error("without a detail or a row, these keys do nothing")
	}
	app.press("m")
	app.expectScreen("still loading this PR")
}

func TestPrViewCommentPostsToTheConversation(t *testing.T) {
	app := prTestApp(t, prDetail(7))
	app.github.REST("POST", "repos/o/r/issues/7/comments", ghtest.Reply(201, map[string]any{"id": 1}))
	app.press("c")
	app.expectScreen("Comment on #7")
	app.send(tea.PasteMsg{Content: "Looks good"})
	app.press("ctrl+s")
	app.expectScreen("comment posted")

	found := app.github.Find("repos/o/r/issues/7/comments")
	if len(found) != 1 || !strings.Contains(found[0].Body, `"body":"Looks good"`) {
		t.Fatalf("comment request = %+v", found)
	}
}

func TestPrViewReview(t *testing.T) {
	tests := []struct {
		name    string
		pending bool
		// Menu keys that pick the verdict, then the summary typed into the composer.
		menu string
		// The menu item sends the review itself, with no composer.
		direct   bool
		summary  string
		required bool
		mutation string
		event    string
		body     string
		toast    string
	}{
		{name: "approve with the default comment", menu: "j", mutation: "addPullRequestReview", event: "APPROVE", body: "LGTM", toast: "review submitted: Approve"},
		{name: "request changes", menu: "jj", summary: "please fix", mutation: "addPullRequestReview", event: "REQUEST_CHANGES", body: "please fix", toast: "review submitted: Request changes"},
		{name: "comment needs a summary", required: true, summary: "a summary", mutation: "addPullRequestReview", event: "COMMENT", body: "a summary", toast: "review submitted: Comment"},
		{name: "comment with pending comments may be empty", pending: true, menu: "j", mutation: "submitPullRequestReview", event: "COMMENT", toast: "review submitted: Comment"},
		{name: "pending comments go out without a summary", pending: true, direct: true, mutation: "submitPullRequestReview", event: "COMMENT", toast: "review submitted: 1 comment"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			detail := prDetail(7)
			if test.pending {
				comment := prComment("octocat", "inline note")
				comment["state"] = "PENDING"
				detail["pendingReviews"] = prNodes(map[string]any{"id": "REVIEW_1"})
				detail["reviewThreads"] = prNodes(prThread("T1", "main.go", 3, comment))
			}
			app := prTestApp(t, detail)
			prAnswer(app.github, test.mutation)
			app.press("a")
			app.expectScreen("Comment", "Approve", "Request changes")
			app.press(test.menu, "enter")
			if test.required {
				app.press("ctrl+s")
				if app.overlay == nil {
					t.Fatal("an empty comment review without inline comments must not be sent")
				}
			}
			if test.summary != "" {
				app.send(tea.PasteMsg{Content: test.summary})
			}
			if !test.direct {
				app.press("ctrl+s")
			}
			app.expectScreen(test.toast)

			variables := prVariables(t, app.github, test.mutation)
			if variables["event"] != test.event || variables["body"] != test.body {
				t.Errorf("variables = %v, want event %s and body %q", variables, test.event, test.body)
			}
			wantID := "PR_7"
			if test.pending {
				wantID = "REVIEW_1"
			}
			if variables["id"] != wantID {
				t.Errorf("review sent for %v, want %s", variables["id"], wantID)
			}
		})
	}
}

func TestPrViewReviewTitlesAndDiscard(t *testing.T) {
	app := prTestApp(t, prDetail(7))
	app.press("a", "enter")
	app.expectScreen("Comment · review summary (required without inline comments)")
	app.press("esc")

	detail := prDetail(8)
	comment := prComment("octocat", "inline note")
	comment["state"] = "PENDING"
	detail["pendingReviews"] = prNodes(map[string]any{"id": "REVIEW_1"})
	detail["reviewThreads"] = prNodes(prThread("T1", "main.go", 3, comment))
	app = prTestApp(t, detail)
	prAnswer(app.github, "deletePullRequestReview")
	app.expectScreen("Files 2 ● 1 pending")
	app.press("a")
	app.expectScreen("Submit review with 1 pending comment", "Submit the 1 comment", "Discard the pending review", "deletes its 1 comment")
	app.press("jjjj", "enter")
	app.expectScreen("pending review discarded")
	if variables := prVariables(t, app.github, "deletePullRequestReview"); variables["id"] != "REVIEW_1" {
		t.Errorf("discarded %v, want REVIEW_1", variables["id"])
	}
}

func TestPrViewWritesFailInReadOnlyMode(t *testing.T) {
	app := prTestApp(t, prDetail(7))
	app.client.ReadOnly = true
	app.press("a", "j", "enter", "ctrl+s")
	app.expectScreen("read-only mode")
	if !app.toast.failure {
		t.Error("a refused write should be reported as a failure")
	}
}

func prThreadsDetail() map[string]any {
	detail := prDetail(7)
	reply := prComment("bob", "agreed")
	reply["createdAt"] = time.Now().Add(-10 * time.Minute)
	pending := prComment("octocat", "my draft")
	pending["state"] = "PENDING"
	resolved := prThread("T_RESOLVED", "docs/readme.md", 4, prComment("carol", "🟡 Minor · nit\n\ntypo"))
	resolved["isResolved"], resolved["resolvedBy"] = true, map[string]any{"login": "carol"}
	outdated := prThread("T_OUTDATED", "old.go", 0, prComment("dave", "🟠 Major · drift\n\nstale"))
	outdated["isOutdated"], outdated["originalLine"] = true, 9
	detail["reviewThreads"] = prNodes(
		prThread("T_OPEN", "internal/some/deeply/nested/directory/with/a/long/name/handler.go", 12,
			prComment("alice", "🔴 Critical · leak\n\nthis leaks a file handle"), reply, prComment("alice", "thanks"), pending),
		resolved,
		outdated,
	)
	return detail
}

func TestPrViewThreadsListAndDetail(t *testing.T) {
	app := prTestApp(t, prThreadsDetail())
	app.expectScreen("Threads 2")
	app.press("3")
	app.expectScreen(
		"handler.go:12", "critical", "@alice · 3 replies", "this leaks a file handle",
		"old.go:9", "major", "outdated",
		"OPEN", "▏ -old", "▏ +new", "● @alice", "↳ @bob", "│ agreed", "pending",
	)
	if strings.Contains(app.screen(), "readme.md") {
		t.Error("resolved threads are hidden by default")
	}
	if !strings.Contains(app.screen(), "…") {
		t.Error("the long path should give up its directory first")
	}

	app.press("j")
	app.expectScreen("OPEN   outdated", "stale")
	app.press("h", "G")
	app.expectScreen("RESOLVED   by @carol", "docs/readme.md:4", "typo")
	app.press("g")
	if app.pullRequest.threadCursor != 0 {
		t.Errorf("g should go to the first thread, cursor = %d", app.pullRequest.threadCursor)
	}

	app.press("ctrl+f")
	if app.pullRequest.threadScroll == 0 {
		t.Error("ctrl+f should scroll the thread")
	}
	app.press("ctrl+b")
	if app.pullRequest.threadScroll != 0 {
		t.Errorf("ctrl+b should scroll back, scroll = %d", app.pullRequest.threadScroll)
	}
}

func TestPrViewThreadsEmptyStates(t *testing.T) {
	detail := prDetail(7)
	resolved := prThread("T1", "main.go", 1, prComment("bob", "done"))
	resolved["isResolved"] = true
	detail["reviewThreads"] = prNodes(resolved)
	app := prTestApp(t, detail)
	app.press("3")
	app.expectScreen("No open threads. h shows the resolved ones.")
	// With nothing in the list, the thread keys have nothing to act on.
	app.press("r", "x", "enter")
	if app.overlay != nil || app.busy != 0 {
		t.Error("thread keys should do nothing without a thread")
	}
	app.press("h")
	app.expectScreen("RESOLVED", "@bob")
}

func TestPrViewThreadActions(t *testing.T) {
	app := prTestApp(t, prThreadsDetail())
	app.github.REST("GET", "repos/o/r/pulls/7", ghtest.Reply(200, "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"))
	for _, mutation := range []string{"addPullRequestReviewThreadReply", "resolveReviewThread", "unresolveReviewThread"} {
		prAnswer(app.github, mutation)
	}
	app.press("3")

	app.press("r")
	app.expectScreen("Reply to @alice · internal/some/deeply/nested/directory/with/a/long/name/handler.go:12")
	app.send(tea.PasteMsg{Content: "on it"})
	app.press("ctrl+s")
	app.expectScreen("reply posted")
	if variables := prVariables(t, app.github, "addPullRequestReviewThreadReply"); variables["id"] != "T_OPEN" || variables["body"] != "on it" {
		t.Errorf("reply variables = %v", variables)
	}

	app.press("x")
	app.expectScreen("thread resolved")
	if variables := prVariables(t, app.github, "resolveReviewThread"); variables["id"] != "T_OPEN" {
		t.Errorf("resolved %v, want T_OPEN", variables["id"])
	}

	app.press("h", "G", "x")
	app.expectScreen("thread reopened")
	if variables := prVariables(t, app.github, "unresolveReviewThread"); variables["id"] != "T_RESOLVED" {
		t.Errorf("reopened %v, want T_RESOLVED", variables["id"])
	}

	// The outdated thread has no line in the current diff.
	app.press("k", "enter")
	if !strings.Contains(app.toast.text, "this thread is outdated") {
		t.Errorf("toast = %q, want the outdated notice", app.toast.text)
	}
	if app.pullRequest.tab != tabThreads {
		t.Error("an outdated thread should not open the diff")
	}
	app.press("g", "enter")
	if app.pullRequest.tab != tabFiles {
		t.Errorf("enter should show the thread in the diff, tab = %d", app.pullRequest.tab)
	}
}

func TestPrViewReplyToAThreadWithoutComments(t *testing.T) {
	detail := prDetail(7)
	detail["reviewThreads"] = prNodes(prThread("T1", "main.go", 5))
	app := prTestApp(t, detail)
	app.press("3", "r")
	app.expectScreen("Reply to the thread · main.go:5")
}

func TestPrViewPeriodicRefresh(t *testing.T) {
	app := prTestApp(t, prDetail(7))
	requests := func() int { return len(app.github.Find("pendingReviews:")) }
	before := requests()

	app.send(tickMsg(time.Now()))
	if requests() != before {
		t.Fatal("a fresh detail should not be fetched again")
	}

	app.data(app.pullRequest.key).detailAt = time.Now().Add(-time.Minute)
	app.press("c")
	app.send(tickMsg(time.Now()))
	if requests() != before {
		t.Fatal("no refresh while an overlay is open")
	}

	app.press("esc")
	app.send(tickMsg(time.Now()))
	if requests() != before+1 {
		t.Fatalf("an old detail should be refetched, requests = %d, want %d", requests(), before+1)
	}
}

var (
	prFakeGhOnce      sync.Once
	prFakeGhDirectory string
)

// prFakeGh puts a gh first on PATH that records its arguments, one call per
// line. The script is shared and run once up front: macOS takes a few hundred
// milliseconds over a new executable's first run, longer than the harness waits.
func prFakeGh(t *testing.T) string {
	t.Helper()
	prFakeGhOnce.Do(func() {
		prFakeGhDirectory = filepath.Join(os.TempDir(), "gh-sift-pr-fake-gh")
		script := filepath.Join(prFakeGhDirectory, "gh")
		_ = os.MkdirAll(prFakeGhDirectory, 0o755)
		_ = os.WriteFile(script, []byte("#!/bin/sh\necho \"$GH_REPO $*\" >> \"$PR_GH_CALLS\"\n"), 0o755)
		_ = exec.Command(script).Run()
	})
	calls := filepath.Join(t.TempDir(), "calls")
	t.Setenv("PR_GH_CALLS", calls)
	t.Setenv("PATH", prFakeGhDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

func prCalls(t *testing.T, calls string) string {
	t.Helper()
	recorded, err := os.ReadFile(calls)
	if err != nil {
		t.Fatalf("gh was never run: %v", err)
	}
	return strings.TrimSpace(string(recorded))
}

func TestPrViewShortPathKeepsTheFileName(t *testing.T) {
	tests := []struct {
		path  string
		limit int
		want  string
	}{
		{"main.go", 20, "main.go"},
		{"internal/ui/prview.go", 15, "…l/ui/prview.go"},
		{"a_very_long_file_name_without_directory.go", 10, "a_very_lo…"},
		{"internal/ui/a_long_file_name.go", 20, "…/a_long_file_name.go"},
	}
	for _, test := range tests {
		if got := shortPath(test.path, test.limit); got != test.want {
			t.Errorf("shortPath(%q, %d) = %q, want %q", test.path, test.limit, got, test.want)
		}
	}
}
