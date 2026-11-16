package gh_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

const pullRequestResponse = `{"data": {"repository": {
  "mergeCommitAllowed": true, "squashMergeAllowed": true, "rebaseMergeAllowed": true,
  "pullRequest": {
    "id": "PR_1", "number": 7, "title": "Add sift", "body": "Body text", "url": "https://github.com/o/r/pull/7",
    "state": "OPEN", "isDraft": false, "isInMergeQueue": false, "isMergeQueueEnabled": true,
    "mergeable": "MERGEABLE", "mergeStateStatus": "CLEAN", "reviewDecision": "REVIEW_REQUIRED",
    "additions": 10, "deletions": 2, "changedFiles": 3, "headRefName": "feature", "baseRefName": "main",
    "headRefOid": "abc123", "updatedAt": "2026-11-13T12:00:00Z", "createdAt": "2026-11-12T12:00:00Z",
    "author": {"login": "octocat"}, "authorAssociation": "MEMBER",
    "repository": {"nameWithOwner": "o/r"},
    "autoMergeRequest": {"mergeMethod": "SQUASH"},
    "comments": {"totalCount": 4},
    "assignees": {"nodes": [{"login": "hubot"}]},
    "labels": {"nodes": [{"name": "bug", "color": "d73a4a"}]},
    "reviewRequests": {"nodes": [
      {"requestedReviewer": {"login": "alice"}},
      {"requestedReviewer": {"name": "core-team"}}
    ]},
    "latestReviews": {"nodes": [{"author": {"login": "copilot[bot]"}, "state": "COMMENTED"}, {"author": null, "state": "APPROVED"}]},
    "pendingReviews": {"nodes": [{"id": "PRR_pending"}]},
    "stackEntry": {"position": 2, "stack": {"id": "S_1", "number": 3, "size": 2, "baseRefName": "main", "entries": {"nodes": [
      {"position": 1, "pullRequest": {"number": 6, "title": "Base", "state": "MERGED", "isDraft": false, "isInMergeQueue": false, "reviewDecision": "APPROVED",
        "commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS"}}}]}}},
      {"position": 2, "pullRequest": {"number": 7, "title": "Add sift", "state": "OPEN", "isDraft": false, "isInMergeQueue": false, "reviewDecision": "",
        "commits": {"nodes": [{"commit": {"statusCheckRollup": null}}]}}}
    ]}}},
    "reviewThreads": {"nodes": [
      {"id": "T_1", "isResolved": true, "resolvedBy": {"login": "alice"}, "isOutdated": false, "path": "main.go",
       "line": 12, "startLine": 10, "originalLine": 12, "diffSide": "RIGHT",
       "hunk": {"nodes": [{"diffHunk": "@@ -1,3 +1,4 @@"}]},
       "comments": {"nodes": [{"id": "C_1", "databaseId": 101, "author": {"login": "alice"}, "body": "Nit", "createdAt": "2026-11-13T10:00:00Z", "state": "SUBMITTED"}]}},
      {"id": "T_2", "isResolved": false, "resolvedBy": null, "isOutdated": true, "path": "old.go",
       "line": 0, "startLine": 0, "originalLine": 4, "diffSide": "LEFT",
       "hunk": {"nodes": []},
       "comments": {"nodes": [{"id": "C_2", "databaseId": 102, "author": {"login": "octocat"}, "body": "Draft", "createdAt": "2026-11-13T11:00:00Z", "state": "PENDING"}]}}
    ]},
    "files": {"pageInfo": {"hasNextPage": true, "endCursor": "CUR1"}, "nodes": [
      {"path": "main.go", "additions": 8, "deletions": 1, "viewerViewedState": "VIEWED"}
    ]},
    "commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "FAILURE", "contexts": {"nodes": [
      {"__typename": "CheckRun", "databaseId": 900, "name": "test", "status": "COMPLETED", "conclusion": "FAILURE",
       "startedAt": "2026-11-13T12:00:00Z", "completedAt": "2026-11-13T12:05:00Z", "detailsUrl": "https://github.com/o/r/actions/runs/5/job/900",
       "title": null, "summary": null, "checkSuite": {"workflowRun": {"databaseId": 5, "event": "pull_request", "workflow": {"name": "CI"}}}},
      {"__typename": "CheckRun", "databaseId": 901, "name": "codecov", "status": "COMPLETED", "conclusion": "NEUTRAL",
       "startedAt": "2026-11-13T12:00:00Z", "completedAt": "2026-11-13T12:01:00Z", "detailsUrl": "https://codecov.io",
       "title": "Coverage 91%", "summary": "Up 2%", "checkSuite": {"workflowRun": null}},
      {"__typename": "StatusContext", "context": "ci/legacy", "state": "PENDING", "createdAt": "2026-11-13T12:00:00Z", "targetUrl": "https://ci.example"},
      {"__typename": "StatusContext", "context": "ci/done", "state": "SUCCESS", "createdAt": "2026-11-13T12:00:00Z", "targetUrl": "https://ci.example/2"}
    ]}}}}]},
    "timelineItems": {"nodes": [
      {"__typename": "IssueComment", "author": {"login": "bob"}, "body": "LGTM?", "createdAt": "2026-11-13T09:00:00Z"},
      {"__typename": "PullRequestReview", "author": {"login": "alice"}, "body": "Looks fine", "state": "APPROVED", "createdAt": "2026-11-13T09:10:00Z"},
      {"__typename": "PullRequestReview", "author": {"login": "octocat"}, "body": "", "state": "PENDING", "createdAt": "2026-11-13T09:11:00Z"},
      {"__typename": "HeadRefForcePushedEvent", "actor": {"login": "octocat"}, "createdAt": "2026-11-13T09:20:00Z"},
      {"__typename": "PullRequestCommit", "commit": {"abbreviatedOid": "abc123", "messageHeadline": "Fix it", "committedDate": "2026-11-13T09:30:00Z"}},
      {"__typename": "MergedEvent", "actor": {"login": "octocat"}, "createdAt": "2026-11-13T09:40:00Z"},
      {"__typename": "ReviewRequestedEvent", "actor": {"login": "octocat"}, "createdAt": "2026-11-13T09:50:00Z", "requestedReviewer": {"name": "core-team"}},
      {"__typename": "ClosedEvent", "actor": null, "createdAt": "2026-11-13T10:00:00Z"},
      {"__typename": "ReopenedEvent", "actor": {"login": "octocat"}, "createdAt": "2026-11-13T10:10:00Z"},
      {"__typename": "LabeledEvent", "actor": {"login": "octocat"}, "createdAt": "2026-11-13T10:20:00Z"}
    ]}
  }
}}}`

const moreFilesResponse = `{"data": {"repository": {"pullRequest": {"files": {
  "pageInfo": {"hasNextPage": false, "endCursor": "CUR2"},
  "nodes": [{"path": "docs/README.md", "additions": 2, "deletions": 1, "viewerViewedState": "DISMISSED"}]
}}}}}`

func TestPullRequestConvertsEveryPart(t *testing.T) {
	server, client := ghtest.New(t)
	server.GraphQL("pullRequest(number: $number)", ghtest.Reply(200, pullRequestResponse))
	server.GraphQL("files(first: 100, after: $after)", ghtest.Reply(200, moreFilesResponse))

	pullRequest, err := client.PullRequest("o/r", 7)
	if err != nil {
		t.Fatal(err)
	}
	if variables := server.Find("pullRequest(number: $number)")[0].Variables; variables["owner"] != "o" || variables["name"] != "r" || variables["number"] != float64(7) {
		t.Errorf("variables = %v", variables)
	}
	if after := server.Find("after: $after")[0].Variables["after"]; after != "CUR1" {
		t.Errorf("the second files page should start after CUR1, got %v", after)
	}

	if pullRequest.Title != "Add sift" || pullRequest.Author != "octocat" || pullRequest.Repository != "o/r" || pullRequest.Body != "Body text" ||
		pullRequest.HeadRefOid != "abc123" || pullRequest.ChangedFiles != 3 || !pullRequest.MergeQueueEnabled {
		t.Errorf("row fields = %+v", pullRequest.PullRequestRow)
	}
	if pullRequest.CommentCount != 4 || pullRequest.AutoMergeMethod != "SQUASH" || pullRequest.PendingReviewID != "PRR_pending" {
		t.Errorf("comments %d, auto-merge %q, pending %q", pullRequest.CommentCount, pullRequest.AutoMergeMethod, pullRequest.PendingReviewID)
	}
	if want := []string{"--squash", "--merge", "--rebase"}; !reflect.DeepEqual(pullRequest.MergeMethods, want) {
		t.Errorf("merge methods = %v, want %v", pullRequest.MergeMethods, want)
	}
	if want := []string{"alice", "core-team"}; !reflect.DeepEqual(pullRequest.ReviewRequests, want) {
		t.Errorf("review requests = %v", pullRequest.ReviewRequests)
	}
	// Bots lose their [bot] suffix and deleted accounts become ghost.
	if want := []gh.Review{{Author: "copilot", State: "COMMENTED"}, {Author: "ghost", State: "APPROVED"}}; !reflect.DeepEqual(pullRequest.LatestReviews, want) {
		t.Errorf("latest reviews = %+v", pullRequest.LatestReviews)
	}
	if pullRequest.StackPosition != 2 || pullRequest.Stack == nil || len(pullRequest.Stack.Entries) != 2 {
		t.Fatalf("stack = %d %+v", pullRequest.StackPosition, pullRequest.Stack)
	}
	if below := pullRequest.Stack.Neighbour(2, -1); below == nil || below.Number != 6 || below.ChecksState != "SUCCESS" {
		t.Errorf("entry below = %+v", below)
	}
	if pullRequest.Stack.Entries[1].ChecksState != "" {
		t.Error("an entry without a rollup has no checks state")
	}

	if len(pullRequest.Threads) != 2 {
		t.Fatalf("threads = %+v", pullRequest.Threads)
	}
	resolved, outdated := pullRequest.Threads[0], pullRequest.Threads[1]
	if resolved.ResolvedBy != "alice" || resolved.DiffHunk != "@@ -1,3 +1,4 @@" || resolved.StartLine != 10 || resolved.Comments[0].DatabaseID != 101 || resolved.Comments[0].Pending {
		t.Errorf("resolved thread = %+v", resolved)
	}
	if outdated.ResolvedBy != "" || outdated.DiffHunk != "" || !outdated.IsOutdated || outdated.Side != "LEFT" || !outdated.Comments[0].Pending {
		t.Errorf("outdated thread = %+v", outdated)
	}
	if pullRequest.UnresolvedThreads() != 1 || pullRequest.PendingComments() != 1 {
		t.Errorf("unresolved %d, pending %d", pullRequest.UnresolvedThreads(), pullRequest.PendingComments())
	}

	if want := []gh.FileSummary{
		{Path: "main.go", Additions: 8, Deletions: 1, ViewedState: "VIEWED"},
		{Path: "docs/README.md", Additions: 2, Deletions: 1, ViewedState: "DISMISSED"},
	}; !reflect.DeepEqual(pullRequest.Files, want) {
		t.Errorf("files = %+v", pullRequest.Files)
	}

	if len(pullRequest.Checks) != 4 {
		t.Fatalf("checks = %+v", pullRequest.Checks)
	}
	job, app, pending, status := pullRequest.Checks[0], pullRequest.Checks[1], pullRequest.Checks[2], pullRequest.Checks[3]
	if !job.IsJob() || job.RunID != 5 || job.JobID != 900 || job.Workflow != "CI" || job.Event != "pull_request" || !job.Failed() {
		t.Errorf("job = %+v", job)
	}
	if app.IsJob() || app.Summary != "Coverage 91%\n\nUp 2%" || !app.Skipped() || app.Failed() {
		t.Errorf("third-party check = %+v", app)
	}
	if pending.Status != "IN_PROGRESS" || pending.Conclusion != "" || !pending.Running() || pending.Failed() {
		t.Errorf("pending status = %+v", pending)
	}
	if status.Name != "ci/done" || status.Status != "COMPLETED" || status.Conclusion != "SUCCESS" || status.URL != "https://ci.example/2" {
		t.Errorf("finished status = %+v", status)
	}

	var kinds []string
	for _, item := range pullRequest.Timeline {
		kinds = append(kinds, item.Kind+":"+item.Author+":"+item.Body)
	}
	want := []string{
		"comment:bob:LGTM?", "review:alice:Looks fine", "force-push:octocat:", "commit:abc123:Fix it",
		"merged:octocat:", "review-requested:octocat:core-team", "closed:ghost:", "reopened:octocat:",
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("timeline = %v\nwant %v", kinds, want)
	}
	if commit := pullRequest.Timeline[3]; !commit.CreatedAt.Equal(time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)) {
		t.Errorf("a commit is placed by its commit date, got %v", commit.CreatedAt)
	}
}

func TestPullRequestErrors(t *testing.T) {
	server, client := ghtest.New(t)
	server.GraphQL("pullRequest(number: $number)", ghtest.Reply(200, map[string]any{"repository": map[string]any{"pullRequest": nil}}))
	if _, err := client.PullRequest("o/r", 9); err == nil || err.Error() != "o/r#9 not found" {
		t.Errorf("missing PR error = %v", err)
	}

	server.GraphQL("pullRequest(number: $number)", ghtest.Reply(502, "bad gateway"))
	if _, err := client.PullRequest("o/r", 9); err == nil {
		t.Error("a failed query should be an error")
	}

	server.GraphQL("pullRequest(number: $number)", ghtest.Reply(200, pullRequestResponse))
	server.GraphQL("after: $after", ghtest.Reply(500, "boom"))
	if _, err := client.PullRequest("o/r", 7); err == nil {
		t.Error("a failed second files page should be an error")
	}
}

func TestDiff(t *testing.T) {
	const unified = "diff --git a/main.go b/main.go\n@@ -1 +1 @@\n-a\n+b\n"
	server, client := ghtest.New(t)
	server.REST("GET", "repos/o/r/pulls/7", ghtest.Reply(200, unified))
	diff, err := client.Diff("o/r", 7)
	if err != nil || diff != unified {
		t.Fatalf("diff = %q, %v", diff, err)
	}

	server.REST("GET", "repos/o/r/pulls/7", ghtest.Reply(500, `{"message": "Server Error"}`))
	if _, err := client.Diff("o/r", 7); err == nil || strings.Contains(err.Error(), "files") {
		t.Errorf("an error other than too large is returned as is, got %v", err)
	}
}

func TestDiffStitchesTooLargeDiffsFromFiles(t *testing.T) {
	server, client := ghtest.New(t)
	server.REST("GET", "repos/o/r/pulls/7", ghtest.Reply(406, `{"message": "Sorry, the diff exceeded the maximum number of files", "errors": [{"code": "too_large"}]}`))
	firstPage := make([]map[string]any, 100)
	firstPage[0] = map[string]any{"filename": "new.go", "status": "added", "patch": "@@ -0,0 +1 @@\n+new"}
	firstPage[1] = map[string]any{"filename": "gone.go", "status": "removed", "patch": "@@ -1 +0,0 @@\n-gone"}
	firstPage[2] = map[string]any{"filename": "moved.go", "previous_filename": "old.go", "status": "renamed", "patch": ""}
	firstPage[3] = map[string]any{"filename": "logo.png", "status": "modified", "patch": ""}
	for index := 4; index < 100; index++ {
		firstPage[index] = map[string]any{"filename": "same.go", "status": "modified", "patch": "@@ -1 +1 @@\n-a\n+b"}
	}
	server.REST("GET", "repos/o/r/pulls/7/files?per_page=100&page=1", ghtest.Reply(200, firstPage))
	server.REST("GET", "repos/o/r/pulls/7/files?per_page=100&page=2", ghtest.Reply(200, []map[string]any{
		{"filename": "last.go", "status": "modified", "patch": "@@ -1 +1 @@\n-x\n+y"},
	}))

	diff, err := client.Diff("o/r", 7)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"diff --git a/new.go b/new.go\nnew file mode 100644\n@@ -0,0 +1 @@\n+new\n",
		"diff --git a/gone.go b/gone.go\ndeleted file mode 100644\n",
		"diff --git a/old.go b/moved.go\nrename from old.go\nrename to moved.go\n\n",
		"Binary files a/logo.png and b/logo.png differ\n",
		"diff --git a/last.go b/last.go\n@@ -1 +1 @@\n-x\n+y\n",
	} {
		if !strings.Contains(diff, expected) {
			t.Errorf("stitched diff lacks %q", expected)
		}
	}
	if strings.Contains(diff, "Binary files a/old.go") {
		t.Error("a pure rename has no patch but is not binary")
	}

	server.REST("GET", "repos/o/r/pulls/7/files", ghtest.Reply(500, `{"message": "boom"}`))
	if _, err := client.Diff("o/r", 7); err == nil {
		t.Error("a failed files page should be an error")
	}
}

func TestPullRequestForBranch(t *testing.T) {
	cases := []struct {
		name   string
		nodes  string
		number int
		fails  bool
	}{
		{"the open one wins", `[{"number": 3, "state": "CLOSED"}, {"number": 5, "state": "OPEN"}]`, 5, false},
		{"else the most recent", `[{"number": 3, "state": "MERGED"}, {"number": 2, "state": "CLOSED"}]`, 3, false},
		{"none", `[]`, 0, true},
	}
	for _, each := range cases {
		t.Run(each.name, func(t *testing.T) {
			server, client := ghtest.New(t)
			server.GraphQL("pullRequests(headRefName: $branch", ghtest.Reply(200, `{"data": {"repository": {"pullRequests": {"nodes": `+each.nodes+`}}}}`))
			number, err := client.PullRequestForBranch("o/r", "feature")
			if number != each.number || (err != nil) != each.fails {
				t.Errorf("number = %d, err = %v", number, err)
			}
			if branch := server.Find("headRefName")[0].Variables["branch"]; branch != "feature" {
				t.Errorf("branch variable = %v", branch)
			}
		})
	}

	server, client := ghtest.New(t)
	server.GraphQL("headRefName", ghtest.Reply(500, "boom"))
	if _, err := client.PullRequestForBranch("o/r", "feature"); err == nil {
		t.Error("a failed query should be an error")
	}
}

func TestAttributes(t *testing.T) {
	server, client := ghtest.New(t)
	server.REST("GET", "repos/o/r/contents/.gitattributes?ref=abc", ghtest.Reply(200, "*.pb.go linguist-generated\n"))
	text, err := client.Attributes("o/r", "abc")
	if err != nil || text != "*.pb.go linguist-generated\n" {
		t.Errorf("attributes = %q, %v", text, err)
	}
	if accept := server.Requests[0]; accept.Path != "repos/o/r/contents/.gitattributes?ref=abc" {
		t.Errorf("path = %q", accept.Path)
	}

	server.REST("GET", "repos/o/r/contents", ghtest.Reply(500, `{"message": "boom"}`))
	if _, err := client.Attributes("o/r", "abc"); err == nil {
		t.Error("only a 404 means there is no .gitattributes")
	}
}
