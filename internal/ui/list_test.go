package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/chrismatix/gh-sift/internal/config"
	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

func listTimestamp(age time.Duration) string {
	return time.Now().Add(-age).UTC().Format(time.RFC3339)
}

// listPullRequestNode is one search result, shaped like searchPullRequestsQuery's nodes.
func listPullRequestNode(repository string, number int, title, state string) map[string]any {
	return map[string]any{
		"id": fmt.Sprintf("PR_%d", number), "number": number, "title": title,
		"url":   fmt.Sprintf("https://github.com/%s/pull/%d", repository, number),
		"state": state, "isDraft": false, "isInMergeQueue": false,
		"mergeable": "MERGEABLE", "reviewDecision": "REVIEW_REQUIRED",
		"additions": 120, "deletions": 8,
		"headRefName": fmt.Sprintf("feature-%d", number), "baseRefName": "main",
		"updatedAt": listTimestamp(2 * time.Hour), "createdAt": listTimestamp(72 * time.Hour),
		"author": map[string]any{"login": "octocat"}, "authorAssociation": "MEMBER",
		"repository":    map[string]any{"nameWithOwner": repository},
		"comments":      map[string]any{"totalCount": 2},
		"reviewThreads": map[string]any{"totalCount": 1},
		"reviews":       map[string]any{"totalCount": 1},
		"assignees":     map[string]any{"nodes": []any{map[string]any{"login": "hubot"}}},
		"labels":        map[string]any{"nodes": []any{map[string]any{"name": "bug", "color": "d73a4a"}}},
		"commits":       map[string]any{"nodes": []any{map[string]any{"commit": map[string]any{"statusCheckRollup": map[string]any{"state": "SUCCESS"}}}}},
	}
}

// listStacked puts a node at a position of the uploader stack: #100 at the
// bottom, then #101 and #102. #100 is never a row of the list.
func listStacked(node map[string]any, position int) map[string]any {
	var entries []any
	for index, title := range []string{"Extract the uploader", "Add retry to the uploader", "Resume interrupted uploads"} {
		entries = append(entries, map[string]any{"position": index + 1, "pullRequest": map[string]any{
			"number": 100 + index, "title": title, "state": "OPEN",
		}})
	}
	node["stackEntry"] = map[string]any{"position": position, "stack": map[string]any{
		"id": "STACK_1", "number": 1, "size": 3, "baseRefName": "main", "entries": map[string]any{"nodes": entries},
	}}
	return node
}

func listPullRequestNodes() []any {
	merged := listPullRequestNode("octo/gadgets", 7, "Fix the flaky clock test", "MERGED")
	merged["reviewDecision"] = "APPROVED"
	merged["comments"] = map[string]any{"totalCount": 0}
	merged["reviewThreads"] = map[string]any{"totalCount": 0}
	return []any{
		listStacked(listPullRequestNode("octo/widgets", 101, "Add retry to the uploader", "OPEN"), 2),
		listStacked(listPullRequestNode("octo/widgets", 102, "Resume interrupted uploads", "OPEN"), 3),
		merged,
	}
}

func listIssueNodes() []any {
	issue := func(number int, title, state, body string) map[string]any {
		return map[string]any{
			"number": number, "title": title, "body": body, "state": state,
			"url":       fmt.Sprintf("https://github.com/octo/widgets/issues/%d", number),
			"updatedAt": listTimestamp(5 * time.Hour), "createdAt": listTimestamp(240 * time.Hour),
			"author":     map[string]any{"login": "monalisa"},
			"repository": map[string]any{"nameWithOwner": "octo/widgets"},
			"comments":   map[string]any{"totalCount": 4},
			"reactions":  map[string]any{"totalCount": 9},
			"assignees":  map[string]any{"nodes": []any{map[string]any{"login": "octocat"}}},
			"labels":     map[string]any{"nodes": []any{map[string]any{"name": "upload", "color": "0e8a16"}}},
		}
	}
	return []any{
		issue(55, "Uploader drops files over 2 GB", "OPEN", "Files over **2 GB** never finish."),
		issue(54, "Progress bar stalls at 99%", "CLOSED", ""),
	}
}

func listSearchPage(nodes []any, hasNextPage bool, endCursor string) map[string]any {
	return map[string]any{"search": map[string]any{
		"issueCount": len(nodes),
		"pageInfo":   map[string]any{"hasNextPage": hasNextPage, "endCursor": endCursor},
		"nodes":      nodes,
	}}
}

// listServe answers PR searches with pullRequests and issue searches with issues.
func listServe(app *testApp, pullRequests, issues []any) {
	app.github.GraphQL("search(", func(request ghtest.Request) (int, any) {
		if strings.HasPrefix(request.Variables["query"].(string), "is:issue") {
			return 200, listSearchPage(issues, false, "")
		}
		return 200, listSearchPage(pullRequests, false, "")
	})
}

// listDetailNode is the PR detail for a list row, shaped like pullRequestQuery.
func listDetailNode(number int) map[string]any {
	for _, each := range listPullRequestNodes() {
		node := each.(map[string]any)
		if node["number"] != number {
			continue
		}
		node["title"] = node["title"].(string) + " (fresh)"
		node["body"] = "Retries every upload **three** times."
		node["headRefOid"] = fmt.Sprintf("sha%d", number)
		node["mergeStateStatus"] = "CLEAN"
		node["latestReviews"] = map[string]any{"nodes": []any{map[string]any{"author": map[string]any{"login": "hubot"}, "state": "APPROVED"}}}
		node["reviewRequests"] = map[string]any{"nodes": []any{map[string]any{"requestedReviewer": map[string]any{"login": "monalisa"}}}}
		return node
	}
	return listStacked(listPullRequestNode("octo/widgets", number, "Extract the uploader", "OPEN"), 1)
}

func listServeDetail(app *testApp) {
	app.github.GraphQL("pullRequest(number:", func(request ghtest.Request) (int, any) {
		number := int(request.Variables["number"].(float64))
		return 200, map[string]any{"repository": map[string]any{"squashMergeAllowed": true, "pullRequest": listDetailNode(number)}}
	})
}

// listTestConfig closes the preview: its delayed fetch costs every move a dropped timer.
func listTestConfig() config.Config {
	cfg := config.Default()
	cfg.Defaults.Preview.Open = false
	return cfg
}

// listApp is the app on the PR list with the fixture rows loaded.
func listApp(t *testing.T, options Options) *testApp {
	t.Helper()
	if options.Config.PRSections == nil {
		options.Config = listTestConfig()
	}
	app := newTestApp(t, options)
	listServe(app, listPullRequestNodes(), listIssueNodes())
	listServeDetail(app)
	app.press("R")
	return app
}

// listFakeGh puts a gh on PATH that records each call as "GH_REPO args (dir)",
// and fails with failure as its output when that is set.
func listFakeGh(t *testing.T, failure string) func() []string {
	t.Helper()
	directory := t.TempDir()
	calls := filepath.Join(directory, "calls")
	script := "#!/bin/sh\necho \"$GH_REPO $* ($(pwd))\" >> " + calls + "\n"
	if failure != "" {
		script += "echo '" + failure + "'\nexit 1\n"
	}
	if err := os.WriteFile(filepath.Join(directory, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+":"+os.Getenv("PATH"))
	return func() []string {
		contents, _ := os.ReadFile(calls)
		return strings.Split(strings.TrimSpace(string(contents)), "\n")
	}
}

func listQueries(app *testApp) []string {
	var queries []string
	for _, request := range app.github.Find("search(") {
		queries = append(queries, request.Variables["query"].(string))
	}
	return queries
}

func TestListShowsTheSearchResults(t *testing.T) {
	app := listApp(t, Options{})
	app.expectScreen(
		"My Pull Requests (3)", "Needs My Review (3)", "Involved (3)",
		"octo/widgets #101 by @octocat", "Add retry to the uploader", "Resume interrupted uploads",
		"octo/gadgets #7", "2/3", "+120 -8", "PRs", "Issues", "@octocat", "1/3", "updated",
	)
	queries := listQueries(app)
	if !strings.Contains(strings.Join(queries, "\n"), "is:pr archived:false is:open review-requested:@me sort:updated") {
		t.Errorf("the sections should search with gh-dash's wrapping: %q", queries)
	}
}

func TestListNavigation(t *testing.T) {
	app := listApp(t, Options{})
	steps := []struct {
		keys    string
		section int
		cursor  int
	}{
		{"j", 0, 1}, {"j", 0, 2}, {"j", 0, 2}, {"k", 0, 1}, {"G", 0, 2}, {"g", 0, 0},
		{"l", 1, 0}, {"l", 2, 0}, {"l", 0, 0}, {"h", 2, 0},
	}
	for _, step := range steps {
		app.press(step.keys)
		if app.list.prIndex != step.section || app.list.current().cursor != step.cursor {
			t.Fatalf("after %q: section %d cursor %d, want %d %d", step.keys, app.list.prIndex, app.list.current().cursor, step.section, step.cursor)
		}
	}
	app.expectScreen("3/3")
}

func TestListLoadsTheNextPageAtTheBottom(t *testing.T) {
	app := newTestApp(t, Options{})
	nodes := listPullRequestNodes()
	app.github.GraphQL("search(", func(request ghtest.Request) (int, any) {
		if request.Variables["after"] == "CURSOR_2" {
			return 200, listSearchPage(nodes[2:], false, "")
		}
		return 200, listSearchPage(nodes[:2], true, "CURSOR_2")
	})
	app.press("r")
	if count := app.list.current().count(); count != 2 {
		t.Fatalf("first page has %d rows", count)
	}
	app.press("j")
	if count := app.list.current().count(); count != 3 {
		t.Fatalf("moving onto the last row should append the next page, have %d rows", count)
	}
	app.expectScreen("Fix the flaky clock test")
}

func TestListShowsALoadingLineWhileTheNextPageLoads(t *testing.T) {
	app := newTestApp(t, Options{})
	nodes := listPullRequestNodes()
	app.github.GraphQL("search(", ghtest.Reply(200, listSearchPage(nodes[:2], true, "CURSOR_2")))
	app.press("r")
	// The fetch command is left unrun, so the page is still on its way.
	if app.list.move(app.App, 1) == nil || !app.list.current().loadingMore {
		t.Fatal("moving onto the last row should start loading the next page")
	}
	app.expectScreen("Loading more…")
	app.send(sectionLoadedMsg{view: config.PRsView, index: 0, generation: app.list.current().generation, appended: true})
	if strings.Contains(app.screen(), "Loading more…") {
		t.Error("the loading line should go once the page arrives")
	}
}

func TestListFlashesTheIconsAReloadChanged(t *testing.T) {
	app := listApp(t, Options{})
	if len(app.list.flashes) != 0 {
		t.Fatalf("a first load has nothing to compare: %v", app.list.flashes)
	}
	nodes := listPullRequestNodes()
	nodes[0].(map[string]any)["reviewDecision"] = "APPROVED"
	listServe(app, nodes, listIssueNodes())
	app.press("r")

	changed := flashKey{pullRequestKey{"octo/widgets", 101}, "review"}
	if len(app.list.flashes) != 1 || app.list.flashes[changed].IsZero() {
		t.Fatalf("only the new review icon should flash: %v", app.list.flashes)
	}
	row := app.list.current().pullRequests[0]
	glyph := app.theme.reviewGlyph(row.ReviewDecision, row.ReviewCount)
	if app.list.flash(app.App, row, "review", glyph) == glyph {
		t.Error("the changed icon should be lit")
	}
	app.list.flashes[changed] = time.Now().Add(-flashDuration - time.Second)
	if app.list.flash(app.App, row, "review", glyph) != glyph {
		t.Error("the flash should end")
	}
}

func TestListDropsASupersededResponse(t *testing.T) {
	app := listApp(t, Options{})
	stale := sectionLoadedMsg{view: config.PRsView, index: 0, generation: app.list.prSections[0].generation - 1}
	app.send(stale)
	if app.list.prSections[0].count() != 3 {
		t.Error("an old generation must not replace the rows")
	}
}

func TestListShowsSearchFailuresAndEmptySections(t *testing.T) {
	app := newTestApp(t, Options{})
	app.expectScreen("Nothing matches these filters")
	app.github.GraphQL("search(", ghtest.Reply(200, `{"errors": [{"message": "API rate limit exceeded"}]}`))
	app.press("r")
	app.expectScreen("API rate limit exceeded")

	app.list.current().err = nil
	app.list.current().loading = true
	app.expectScreen("Fetching…")
}

func TestListSearchEditsTheFilters(t *testing.T) {
	app := listApp(t, Options{})
	app.press("/")
	if !app.list.searching {
		t.Fatal("/ should start a search")
	}
	app.press("ctrl+w")
	app.send(tea.PasteMsg{Content: "label:bug"})
	app.press("enter")
	if app.list.searching || app.list.current().filters != "is:open label:bug" {
		t.Fatalf("filters = %q", app.list.current().filters)
	}
	queries := listQueries(app)
	if last := queries[len(queries)-1]; last != "is:pr archived:false is:open label:bug sort:updated" {
		t.Errorf("searched %q", last)
	}

	app.press("/", "x", "esc")
	if app.list.searching || app.list.current().filters != "is:open label:bug" {
		t.Error("esc should leave the filters alone")
	}
}

func TestListFiltersRenderTemplates(t *testing.T) {
	cfg := config.Default()
	cfg.PRSections = []config.PrsSectionConfig{
		{Title: "Weeks", Filters: `updated:>={{ nowModify "-3w" }}`},
		{Title: "Hours", Filters: `updated:>={{ nowModify "-48h" }}`},
		{Title: "Broken", Filters: `{{ nope`},
	}
	app := newTestApp(t, Options{Config: cfg})
	queries := strings.Join(listQueries(app), "\n")
	for _, want := range []string{
		"updated:>=" + time.Now().AddDate(0, 0, -21).Format("2006-01-02"),
		"updated:>=" + time.Now().Add(-48*time.Hour).Format("2006-01-02"),
		"{{ nope",
	} {
		if !strings.Contains(queries, want) {
			t.Errorf("queries lack %q:\n%s", want, queries)
		}
	}
}

func TestListSmartFiltering(t *testing.T) {
	app := listApp(t, Options{Repository: "octo/widgets"})
	app.expectScreen("is:pr repo:octo/widgets", "octo/widgets")
	queries := listQueries(app)
	if last := queries[len(queries)-1]; !strings.HasPrefix(last, "is:pr archived:false repo:octo/widgets ") {
		t.Errorf("smart filtering should narrow to the repository: %q", last)
	}
	app.press("t")
	queries = listQueries(app)
	if last := queries[len(queries)-1]; strings.Contains(last, "repo:") {
		t.Errorf("t should turn smart filtering off: %q", last)
	}

	outside := listApp(t, Options{})
	outside.press("t")
	outside.expectScreen("smart filtering needs sift")
}

func TestListPreviewShowsTheSelectedPullRequest(t *testing.T) {
	app := listApp(t, Options{Config: config.Default()})
	app.expectScreen("loading…", "Stack · 2 of 3", "feature-101 → main")
	app.send(previewMsg{key: pullRequestKey{"octo/widgets", 101}, request: app.list.previewRequest - 1})
	if len(app.github.Find("pullRequest(number:")) != 0 {
		t.Fatal("a superseded preview must not fetch")
	}
	app.send(previewMsg{key: pullRequestKey{"octo/widgets", 101}, request: app.list.previewRequest})
	app.expectScreen("Add retry to the uploader (fresh)", "ready to merge", "✓ hubot", "○ monalisa")

	app.press("ctrl+d")
	if app.list.previewScroll == 0 {
		t.Error("ctrl+d should scroll the preview")
	}
	app.expectScreen("Retries every upload")
	app.press("ctrl+u")
	if app.list.previewScroll != 0 {
		t.Error("ctrl+u should scroll back")
	}

	app.send(tea.WindowSizeMsg{Width: 220, Height: 40})
	app.expectScreen("│  ")
	app.press("P")
	if !app.list.flipPreview {
		t.Error("P should flip the preview")
	}
	app.expectScreen("━━━━━━━━")
	app.press("p")
	if app.list.previewOpen || strings.Contains(app.screen(), "Retries every upload") {
		t.Error("p should close the preview")
	}
}

func TestListPreviewOfAnIssue(t *testing.T) {
	app := listApp(t, Options{Config: config.Default()})
	app.press("s")
	app.expectScreen("My Issues (2)", "octo/widgets #55 by @monalisa", "Uploader drops files over 2 GB",
		"by @monalisa", "4 comments", "upload", "never finish")
	app.press("j")
	app.expectScreen("Progress bar stalls at 99%")
	app.press("s")
	if app.list.kind != config.PRsView {
		t.Error("s should switch back to PRs")
	}
	app.press("s")
	if count := len(app.github.Find("on Issue")); count != 3 {
		t.Errorf("each issue section should be fetched once, not %d times in all", count)
	}
}

func TestListStatusLines(t *testing.T) {
	app := newTestApp(t, Options{})
	open := gh.PullRequestRow{State: "OPEN", BaseRefName: "main"}
	tests := []struct {
		name   string
		detail gh.PullRequest
		want   []string
	}{
		{"approved and clean", gh.PullRequest{PullRequestRow: listWithRow(open, func(row *gh.PullRequestRow) { row.ReviewDecision, row.MergeStateStatus = "APPROVED", "CLEAN" })},
			[]string{"approved", "ready to merge", "Checks    none"}},
		{"changes requested and conflicting", gh.PullRequest{PullRequestRow: listWithRow(open, func(row *gh.PullRequestRow) { row.ReviewDecision, row.Mergeable = "CHANGES_REQUESTED", "CONFLICTING" }),
			LatestReviews: []gh.Review{{Author: "hubot", State: "CHANGES_REQUESTED"}, {Author: "monalisa", State: "COMMENTED"}}},
			[]string{"changes requested", "conflicts with main", "✗ hubot", "● monalisa"}},
		{"behind with threads", gh.PullRequest{PullRequestRow: listWithRow(open, func(row *gh.PullRequestRow) { row.ReviewDecision, row.MergeStateStatus = "REVIEW_REQUIRED", "BEHIND" }),
			Threads: []gh.Thread{{IsResolved: false}, {IsResolved: true}}},
			[]string{"review required", "1 open thread", "behind main"}},
		{"queued with auto-merge", gh.PullRequest{PullRequestRow: listWithRow(open, func(row *gh.PullRequestRow) { row.IsInMergeQueue = true }), AutoMergeMethod: "SQUASH"},
			[]string{"no review needed", "in the merge queue", "auto-merge on"}},
		{"blocked with people and checks", gh.PullRequest{
			PullRequestRow: listWithRow(open, func(row *gh.PullRequestRow) {
				row.MergeStateStatus, row.Labels, row.Assignees = "BLOCKED", []gh.Label{{Name: "bug", Color: "d73a4a"}}, []string{"octocat", "hubot"}
			}),
			Checks: []gh.Check{{Status: "COMPLETED", Conclusion: "SUCCESS"}, {Status: "COMPLETED", Conclusion: "FAILURE"}, {Status: "IN_PROGRESS"}}},
			[]string{"blocked", "Labels", "bug", "Assignees octocat, hubot", "✓ 1", "✗ 1", "● 1"}},
		{"merged has no merge line", gh.PullRequest{PullRequestRow: gh.PullRequestRow{State: "MERGED"}}, []string{"Review"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			text := listPlain(statusLines(app.App, &test.detail, 80))
			for _, want := range test.want {
				if !strings.Contains(text, want) {
					t.Errorf("status lacks %q:\n%s", want, text)
				}
			}
			if test.detail.State == "MERGED" && strings.Contains(text, "Merge ") {
				t.Errorf("a merged PR needs no merge line:\n%s", text)
			}
		})
	}
}

func listWithRow(row gh.PullRequestRow, change func(row *gh.PullRequestRow)) gh.PullRequestRow {
	change(&row)
	return row
}

func listPlain(lines []string) string {
	var plain []string
	for _, line := range lines {
		plain = append(plain, strings.TrimSpace(ansi.Strip(line)))
	}
	return strings.Join(plain, "\n")
}

// listPressSlow sends one key and waits for its command however long it
// takes: writes that shell out to gh outlive the harness's 100ms.
func listPressSlow(app *testApp, name string) {
	app.t.Helper()
	key, named := namedKey(name)
	if !named {
		key = tea.KeyPressMsg{Code: []rune(name)[0], Text: name}
	}
	_, command := app.Update(key)
	if command != nil {
		app.send(command())
	}
}

// listQuitting reports whether pressing key on the current screen quits.
func listQuitting(app *testApp, key tea.KeyPressMsg) bool {
	_, command := app.Update(key)
	if command == nil {
		return false
	}
	_, quits := command().(tea.QuitMsg)
	return quits
}

// listAction is one list action: keys that open it, text pasted into its
// input (if it has one) and the gh call and toast it should lead to.
type listAction struct {
	keys  []string
	paste string
	call  string
	toast string
}

// listRunActions runs each action on one app, waiting for its gh call.
// Every call costs a real process, so the actions share the app.
func listRunActions(t *testing.T, app *testApp, calls func() []string, actions []listAction) {
	t.Helper()
	for _, action := range actions {
		searches := len(app.github.Find("search("))
		if app.overlay == nil && action.paste == "" && len(action.keys) == 2 {
			app.press(action.keys[0])
			listPressSlow(app, action.keys[1])
		} else {
			app.press(action.keys...)
			app.send(tea.PasteMsg{Content: action.paste})
			listPressSlow(app, "enter")
		}
		got := calls()
		if !strings.HasPrefix(got[len(got)-1], action.call+" (") {
			t.Errorf("%v: gh got %q, want %q", action.keys, got[len(got)-1], action.call)
		}
		app.expectScreen(action.toast)
		if len(app.github.Find("search(")) <= searches {
			t.Errorf("%v: a finished action should refresh the section", action.keys)
		}
	}
}

func TestListPullRequestActionsRunGh(t *testing.T) {
	calls := listFakeGh(t, "")
	app := listApp(t, Options{})
	listRunActions(t, app, calls, []listAction{
		{[]string{"x", "y"}, "", "octo/widgets pr close 101", "closed #101"},
		{[]string{"a"}, "hubot  monalisa", "octo/widgets pr edit 101 --add-assignee hubot,monalisa", "assigned hubot,monalisa"},
		{[]string{"L", "ctrl+u"}, "ux", "octo/widgets pr edit 101 --add-label ux --remove-label bug", "labels updated"},
	})

	// These share close's path to gh; their questions are what differs.
	questions := map[string]string{
		"X": "Are you sure you want to reopen this PR? (y/N)",
		"W": "Are you sure you want to mark this PR as ready? (y/N)",
		"u": "Are you sure you want to update this PR from its base? (y/N)",
		"A": "Unassign #101 (logins, space separated)",
		"L": "Labels of #101 (comma separated)",
	}
	for key, question := range questions {
		app.press(key)
		app.expectScreen(question)
		app.press("esc")
	}
	app.press("A")
	if value := app.overlay.input.Value(); value != "hubot" {
		t.Errorf("unassign should offer the current assignees, got %q", value)
	}
}

func TestListActionsThatChangeNothing(t *testing.T) {
	calls := listFakeGh(t, "")
	app := listApp(t, Options{})
	app.press("x", "n", "L", "enter", "a", "enter")
	if app.overlay != nil {
		t.Error("every overlay should be closed")
	}
	if got := calls(); got[0] != "" {
		t.Errorf("nothing should have run gh: %q", got)
	}
	app.press("C")
	app.expectScreen("no local path for octo/widgets")
}

func TestListCheckoutRunsInTheRepositoryPath(t *testing.T) {
	calls := listFakeGh(t, "")
	checkout := t.TempDir()
	cfg := listTestConfig()
	cfg.RepoPaths = map[string]string{"octo/*": checkout + "/*"}
	if err := os.Mkdir(checkout+"/widgets", 0o755); err != nil {
		t.Fatal(err)
	}
	app := listApp(t, Options{Config: cfg})
	listPressSlow(app, "space")
	if got := calls(); !strings.HasPrefix(got[0], "octo/widgets pr checkout 101 (") || !strings.HasSuffix(got[0], "widgets)") {
		t.Errorf("gh got %q", got)
	}
	app.expectScreen("checked out #101 in")
}

func TestListActionFailures(t *testing.T) {
	listFakeGh(t, "GraphQL: Pull request is in clean status (updatePullRequestBranch)")
	app := listApp(t, Options{})
	app.press("u")
	listPressSlow(app, "y")
	app.expectScreen("GraphQL: Pull request is in clean status")

	app.client.ReadOnly = true
	app.toast = toast{}
	app.expectScreen("read-only")
	app.press("x", "y")
	app.expectScreen("read-only mode (SIFT_READ_ONLY is set)")
}

func TestListApproveAndComment(t *testing.T) {
	app := listApp(t, Options{})
	app.github.GraphQL("addPullRequestReview", ghtest.Reply(200, map[string]any{}))
	app.github.GraphQL("submitPullRequestReview", ghtest.Reply(200, map[string]any{}))
	app.github.REST("POST", "repos/octo/widgets/issues/101/comments", ghtest.Reply(201, map[string]any{"id": 1}))

	app.press("v")
	app.expectScreen("Approve #101", "LGTM")
	app.press("enter")
	approvals := app.github.Find("addPullRequestReview")
	if len(approvals) != 1 || approvals[0].Variables["body"] != "LGTM" || approvals[0].Variables["id"] != "PR_101" {
		t.Fatalf("approval = %+v", approvals)
	}
	app.expectScreen("approved #101")

	app.data(pullRequestKey{"octo/widgets", 101}).detail = &gh.PullRequest{PendingReviewID: "PRR_9"}
	app.press("v", "enter")
	if pending := app.github.Find("submitPullRequestReview"); len(pending) != 1 || pending[0].Variables["id"] != "PRR_9" {
		t.Errorf("approving over a pending review should submit it: %+v", pending)
	}

	app.press("c")
	app.expectScreen("Comment on #101", "ctrl+s send")
	app.press("ctrl+s")
	if app.overlay == nil {
		t.Fatal("an empty comment should not be sent")
	}
	app.send(tea.PasteMsg{Content: "Ship it"})
	app.press("ctrl+s")
	comments := app.github.Find("repos/octo/widgets/issues/101/comments")
	if len(comments) != 1 || !strings.Contains(comments[0].Body, "Ship it") {
		t.Fatalf("comment = %+v", comments)
	}
	app.expectScreen("commented on #101")
}

func TestListOpensThePullRequest(t *testing.T) {
	app := listApp(t, Options{})
	app.github.REST("GET", "repos/octo/widgets/pulls/101", ghtest.Reply(200, "diff --git a/up.go b/up.go\n--- a/up.go\n+++ b/up.go\n@@ -1 +1 @@\n-old\n+new\n"))
	app.press("enter")
	if app.App.screen != screenPullRequest || app.pullRequest.key != (pullRequestKey{"octo/widgets", 101}) {
		t.Fatalf("enter should open #101, screen %v key %v", app.App.screen, app.pullRequest.key)
	}
	if len(app.github.Find("pullRequest(number:")) == 0 {
		t.Error("opening should fetch the detail")
	}
	app.press("esc")
	if app.App.screen != screenList {
		t.Fatal("esc should go back to the list")
	}

	app.press("d")
	if app.App.screen != screenPullRequest || app.pullRequest.tab != tabFiles {
		t.Fatal("d should open the diff")
	}
	if len(app.github.Find("repos/octo/widgets/pulls/101")) == 0 {
		t.Error("the diff should be fetched")
	}
	app.press("q")
	if app.App.screen != screenList {
		t.Error("q in a diff opened from the list should go back to it")
	}
}

func TestListMovesThroughTheStack(t *testing.T) {
	app := listApp(t, Options{})
	app.press("K")
	if app.list.current().cursor != 1 {
		t.Fatalf("K should move onto #102, cursor %d", app.list.current().cursor)
	}
	app.press("K")
	app.expectScreen("top of the stack")
	app.press("G", "K")
	app.expectScreen("this PR is not part of a stack")

	app.press("g", "J")
	if app.App.screen != screenPullRequest || app.pullRequest.key.number != 100 {
		t.Fatalf("J from #101 should open #100, which is not a row: %v", app.pullRequest.key)
	}
}

func TestListMerge(t *testing.T) {
	calls := listFakeGh(t, "")
	app := listApp(t, Options{})
	app.press("m")
	app.expectScreen("still loading this PR")
	app.press("m")
	app.expectScreen("Merge stack · #101 is 2 of 3", "Squash and merge")
	listPressSlow(app, "enter")
	if got := calls(); !strings.HasPrefix(got[0], "octo/widgets stack merge 101 --yes --squash") {
		t.Errorf("gh got %q", got)
	}
	app.expectScreen("merged the stack up to #101")
}

func TestListBrowserAndClipboard(t *testing.T) {
	app := listApp(t, Options{})
	opened := filepath.Join(t.TempDir(), "opened")
	browser := filepath.Join(t.TempDir(), "browser")
	if err := os.WriteFile(browser, []byte("#!/bin/sh\necho \"$1\" > "+opened+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_BROWSER", browser)
	listPressSlow(app, "o")
	if contents, _ := os.ReadFile(opened); strings.TrimSpace(string(contents)) != "https://github.com/octo/widgets/pull/101" {
		t.Errorf("browser opened %q", contents)
	}
	app.press("y")
	app.expectScreen("copied #101")
	app.press("Y")
	app.expectScreen("copied https://github.com/octo/widgets/pull/101")

	searches := len(app.github.Find("search("))
	app.press("r")
	if len(app.github.Find("search(")) != searches+1 {
		t.Error("r should refresh the current section only")
	}
}

func TestListKeysOnAnEmptySection(t *testing.T) {
	app := newTestApp(t, Options{Config: listTestConfig()})
	app.press("o", "y", "Y", "enter", "x", "j")
	if app.overlay != nil || app.App.screen != screenList || app.toast.text != "" {
		t.Error("row actions on an empty section should do nothing")
	}
}

func TestListQuit(t *testing.T) {
	app := listApp(t, Options{})
	if !listQuitting(app, tea.KeyPressMsg{Code: 'q', Text: "q"}) {
		t.Error("q should quit")
	}

	cfg := listTestConfig()
	cfg.ConfirmQuit = true
	careful := listApp(t, Options{Config: cfg})
	careful.press("q")
	careful.expectScreen("Really quit?")
	careful.press("n")
	if careful.overlay != nil {
		t.Fatal("n should dismiss the question")
	}
	careful.press("q")
	if !listQuitting(careful, tea.KeyPressMsg{Code: 'y', Text: "y"}) {
		t.Error("y should confirm quitting")
	}
}

func TestListIssueActions(t *testing.T) {
	calls := listFakeGh(t, "")
	app := listApp(t, Options{})
	app.press("s")
	listRunActions(t, app, calls, []listAction{
		{[]string{"X", "y"}, "", "octo/widgets issue reopen 55", "reopen #55 done"},
		{[]string{"A"}, "octocat", "octo/widgets issue edit 55 --remove-assignee octocat", "unassigned octocat"},
		{[]string{"L"}, "bug", "octo/widgets issue edit 55 --add-label bug", "labels updated"},
	})
	app.press("x")
	app.expectScreen("Are you sure you want to close this issue? (y/N)")
	app.press("n", "a")
	app.expectScreen("assign #55 (logins, space separated)")
}

func TestListIssueComment(t *testing.T) {
	calls := listFakeGh(t, "")
	app := listApp(t, Options{})
	app.github.REST("POST", "repos/octo/widgets/issues/55/comments", ghtest.Reply(201, map[string]any{"id": 1}))
	app.press("s", "a", "enter", "L", "enter", "c")
	app.expectScreen("Comment on #55")
	app.send(tea.PasteMsg{Content: "Reproduced on 2.4"})
	app.press("ctrl+d")
	if len(app.github.Find("repos/octo/widgets/issues/55/comments")) != 1 {
		t.Error("ctrl+d should send the comment")
	}
	app.expectScreen("commented on #55")
	if got := calls(); got[0] != "" {
		t.Errorf("empty assign and label inputs should not run gh: %q", got)
	}
	app.press("t")
	app.expectScreen("smart filtering needs sift")
}

func TestListUnsupportedBuiltins(t *testing.T) {
	cfg := listTestConfig()
	cfg.Keybindings.Prs = []config.Keybinding{{Key: "w", Builtin: "watchChecks"}}
	cfg.Keybindings.Issues = []config.Keybinding{{Key: "C", Builtin: "checkout"}}
	app := listApp(t, Options{Config: cfg})
	app.press("w")
	app.expectScreen("watchChecks is not supported yet")
	app.press("s", "C")
	app.expectScreen("checkout is not supported yet")
}

func TestListRefetchesStaleSections(t *testing.T) {
	app := listApp(t, Options{})
	searches := len(app.github.Find("search("))
	app.send(tickMsg(time.Now()))
	if len(app.github.Find("search(")) != searches {
		t.Fatal("fresh sections should not refetch")
	}
	for _, current := range app.list.prSections {
		current.loadedAt = time.Now().Add(-time.Hour)
	}
	app.send(tickMsg(time.Now()))
	if got := len(app.github.Find("search(")) - searches; got != 3 {
		t.Errorf("every stale section should refetch, got %d searches", got)
	}
}

func TestListCachePaintsBeforeTheSearch(t *testing.T) {
	app := listApp(t, Options{})
	again, err := New(Options{Config: listTestConfig(), Client: app.client})
	if err != nil {
		t.Fatal(err)
	}
	if count := again.list.prSections[0].count(); count != 3 {
		t.Errorf("a new start should paint the %d cached rows, has %d", 3, count)
	}
}

func TestListLayouts(t *testing.T) {
	hide := func(column *config.ColumnConfig) { hidden := true; column.Hidden = &hidden }
	tests := []struct {
		name      string
		width     int
		configure func(cfg *config.Config)
		want      []string
		unwanted  []string
	}{
		{"compact without separators", 140, func(cfg *config.Config) {
			cfg.Theme.Ui.Table.Compact, cfg.Theme.Ui.Table.ShowSeparator = true, false
			cfg.PRSections[0].Layout.Base.Hidden = new(bool)
		}, []string{"#101 Add retry to the uploader", "widgets", "Author", "Base", "main"}, []string{"\n  ────"}},
		{"hidden columns", 140, func(cfg *config.Config) {
			layout := &cfg.PRSections[0].Layout
			for _, column := range []*config.ColumnConfig{&layout.State, &layout.NumComments, &layout.ReviewStatus, &layout.Ci, &layout.Lines, &layout.UpdatedAt, &layout.CreatedAt} {
				hide(column)
			}
		}, []string{"Add retry to the uploader"}, []string{"+120", "3d"}},
		{"base branch shown", 140, func(cfg *config.Config) {
			cfg.PRSections[0].Layout.Base.Hidden = new(bool)
		}, []string{"by @octocat · feature-101", "Base"}, nil},
		{"no counts and no theme", 140, func(cfg *config.Config) {
			cfg.Theme = nil
		}, []string{"My Pull Requests (3)"}, nil},
		{"counts turned off", 140, func(cfg *config.Config) {
			cfg.Theme.Ui.SectionsShowCount = false
		}, []string{"My Pull Requests"}, []string{"My Pull Requests (3)"}},
		{"configured limit and widths", 140, func(cfg *config.Config) {
			limit, width := 2, 30
			cfg.PRSections[0].Limit = &limit
			cfg.PRSections[0].Layout.Lines.Width = &width
			cfg.IssuesSections[0].Limit = &limit
		}, []string{"+120 -8"}, nil},
		{"narrow drops columns", 50, func(*config.Config) {}, []string{"Title"}, []string{"3d"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := listTestConfig()
			test.configure(&cfg)
			app := listApp(t, Options{Config: cfg})
			app.send(tea.WindowSizeMsg{Width: test.width, Height: 40})
			app.expectScreen(test.want...)
			for _, text := range test.unwanted {
				if strings.Contains(app.screen(), text) {
					t.Errorf("screen should not show %q:\n%s", text, app.screen())
				}
			}
		})
	}
}

func TestListIssueLayout(t *testing.T) {
	cfg := listTestConfig()
	hidden := true
	cfg.IssuesSections[0].Layout.Reactions.Hidden = &hidden
	cfg.IssuesSections[0].Layout.CreatedAt.Hidden = &hidden
	cfg.Defaults.View = config.IssuesView
	app := listApp(t, Options{Config: cfg})
	app.expectScreen("is:issue is:open author:@me", "Uploader drops files over 2 GB", "4", "5h")
	if strings.Contains(app.screen(), "10d") {
		t.Error("the hidden created column should not show")
	}
}

func TestListWithoutSections(t *testing.T) {
	cfg := listTestConfig()
	cfg.PRSections, cfg.IssuesSections = []config.PrsSectionConfig{}, nil
	app := newTestApp(t, Options{Config: cfg})
	app.press("r", "l", "j", "s", "x")
	if len(app.github.Find("search(")) != 0 || app.overlay != nil {
		t.Error("with no sections there is nothing to fetch or act on")
	}
	app.expectScreen("Nothing matches these filters")
}

func TestListTabsAndSearchBarWhileWorking(t *testing.T) {
	app := newTestApp(t, Options{Config: listTestConfig()})
	app.list.prSections[1].loading = true
	app.list.prSections[1].loadedAt = time.Time{}
	app.expectScreen("Needs My Review " + app.spinner())
	app.press("/")
	app.send(tea.PasteMsg{Content: " draft:true"})
	app.expectScreen("is:pr is:open author:@me draft:true")
	if app.list.current().filters != "is:open author:@me" {
		t.Error("the filters change only on enter")
	}
}

// Before the first resize a real program reports a 0×0 window.
func TestListViewSurvivesATinyWindow(t *testing.T) {
	list := newTestApp(t, Options{})
	diff := diffViewOpen(t, diffViewDefaultFixture())
	for _, app := range []*testApp{list, diff} {
		for _, tab := range []string{"1", "2", "3", "4"} {
			if app == diff {
				app.press(tab)
			}
			for _, width := range []int{0, 1, 2} {
				app.send(tea.WindowSizeMsg{Width: width, Height: width})
				_ = app.View()
			}
		}
	}
}

func TestListShowsAMergeQueueSection(t *testing.T) {
	options := Options{Config: listTestConfig()}
	options.Config.PRSections = []config.PrsSectionConfig{{Title: "Queue", MergeQueue: &config.MergeQueueConfig{Repo: "octo/widgets"}}}
	app := newTestApp(t, options)
	node := listPullRequestNode("octo/widgets", 101, "Add retry to the uploader", "OPEN")
	app.github.GraphQL("mergeQueue(branch:", ghtest.Reply(200, map[string]any{"repository": map[string]any{"mergeQueue": map[string]any{"entries": map[string]any{
		"totalCount": 1, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""},
		"nodes": []any{map[string]any{"position": 1, "state": "MERGEABLE", "enqueuedAt": listTimestamp(5 * time.Minute), "estimatedTimeToMerge": 120, "pullRequest": node}},
	}}}}))
	listServeDetail(app)
	app.press("R")

	app.expectScreen("merge queue octo/widgets → default branch", "1 merging · ~2m", "Add retry to the uploader")
	if len(app.github.Find("search(")) != 0 {
		t.Error("a merge queue section should not search")
	}
	// A preview load replaces the row's fields but must keep its queue place.
	app.list.updateRow(&gh.PullRequest{PullRequestRow: gh.PullRequestRow{Repository: "octo/widgets", Number: 101}})
	if app.list.current().pullRequests[0].Queue == nil {
		t.Error("the queue entry should survive a detail refresh")
	}
	app.press("/")
	if app.list.searching {
		t.Error("a merge queue has no filters to edit")
	}
}
