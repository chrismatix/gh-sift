package gh_test

import (
	"reflect"
	"testing"

	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

const searchPullRequestsResponse = `{"data": {"search": {
  "issueCount": 42,
  "pageInfo": {"hasNextPage": true, "endCursor": "NEXT"},
  "nodes": [
    {"id": "PR_1", "number": 7, "title": "Add sift", "url": "https://github.com/o/r/pull/7", "state": "OPEN",
     "isDraft": true, "isInMergeQueue": false, "mergeable": "CONFLICTING", "reviewDecision": "CHANGES_REQUESTED",
     "additions": 10, "deletions": 2, "headRefName": "feature", "baseRefName": "main",
     "updatedAt": "2026-11-13T12:00:00Z", "createdAt": "2026-11-12T12:00:00Z",
     "author": {"login": "dependabot[bot]"}, "authorAssociation": "NONE",
     "repository": {"nameWithOwner": "o/r"},
     "comments": {"totalCount": 2}, "reviewThreads": {"totalCount": 3}, "reviews": {"totalCount": 1},
     "assignees": {"nodes": [{"login": "alice"}, {"login": "bob"}]},
     "labels": {"nodes": [{"name": "deps", "color": "0366d6"}]},
     "commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "PENDING"}}}]},
     "stackEntry": {"position": 1, "stack": {"id": "S_1", "number": 1, "size": 1, "baseRefName": "main", "entries": {"nodes": [
       {"position": 1, "pullRequest": {"number": 7, "title": "Add sift", "state": "OPEN", "isDraft": true, "isInMergeQueue": false, "reviewDecision": "CHANGES_REQUESTED"}}
     ]}}}},
    {"id": "PR_2", "number": 8, "title": "Unstacked", "url": "https://github.com/o/r/pull/8", "state": "MERGED",
     "author": null, "repository": {"nameWithOwner": "o/r"},
     "comments": {"totalCount": 0}, "reviewThreads": {"totalCount": 0}, "reviews": {"totalCount": 0},
     "assignees": {"nodes": []}, "labels": {"nodes": []}, "commits": {"nodes": []}, "stackEntry": null}
  ]
}}}`

func TestSearchPullRequests(t *testing.T) {
	server, client := ghtest.New(t)
	server.GraphQL("... on PullRequest", ghtest.Reply(200, searchPullRequestsResponse))

	page, err := client.SearchPullRequests("author:@me", 20, "")
	if err != nil {
		t.Fatal(err)
	}
	variables := server.Requests[0].Variables
	if variables["query"] != "is:pr archived:false author:@me sort:updated" || variables["limit"] != float64(20) {
		t.Errorf("variables = %v", variables)
	}
	if _, hasAfter := variables["after"]; hasAfter {
		t.Error("the first page sends no cursor")
	}
	if page.TotalCount != 42 || !page.HasNextPage || page.EndCursor != "NEXT" || len(page.Rows) != 2 {
		t.Fatalf("page = %+v", page)
	}
	stacked, plain := page.Rows[0], page.Rows[1]
	if stacked.Author != "dependabot" || stacked.CommentCount != 5 || stacked.ReviewCount != 1 || stacked.ChecksState != "PENDING" ||
		!stacked.IsDraft || stacked.Mergeable != "CONFLICTING" {
		t.Errorf("stacked row = %+v", stacked)
	}
	if !reflect.DeepEqual(stacked.Assignees, []string{"alice", "bob"}) || !reflect.DeepEqual(stacked.Labels, []gh.Label{{Name: "deps", Color: "0366d6"}}) {
		t.Errorf("assignees %v, labels %v", stacked.Assignees, stacked.Labels)
	}
	if stacked.StackPosition != 1 || stacked.Stack == nil || stacked.Stack.Size != 1 || stacked.Stack.Neighbour(1, 1) != nil {
		t.Errorf("stack = %d %+v", stacked.StackPosition, stacked.Stack)
	}
	if plain.Author != "ghost" || plain.Stack != nil || plain.StackPosition != 0 || plain.ChecksState != "" {
		t.Errorf("plain row = %+v", plain)
	}

	if _, err := client.SearchPullRequests("author:@me", 20, "NEXT"); err != nil {
		t.Fatal(err)
	}
	if after := server.Requests[1].Variables["after"]; after != "NEXT" {
		t.Errorf("after = %v", after)
	}

	server.GraphQL("... on PullRequest", ghtest.Reply(500, "boom"))
	if _, err := client.SearchPullRequests("", 20, ""); err == nil {
		t.Error("a failed search should be an error")
	}
}

func TestSearchIssues(t *testing.T) {
	server, client := ghtest.New(t)
	server.GraphQL("... on Issue", ghtest.Reply(200, `{"data": {"search": {
  "issueCount": 1,
  "pageInfo": {"hasNextPage": false, "endCursor": "END"},
  "nodes": [{"number": 3, "title": "Crash", "body": "It crashes", "url": "https://github.com/o/r/issues/3", "state": "OPEN",
    "updatedAt": "2026-11-13T12:00:00Z", "createdAt": "2026-11-12T12:00:00Z",
    "author": {"login": "carol"}, "repository": {"nameWithOwner": "o/r"},
    "comments": {"totalCount": 6}, "reactions": {"totalCount": 9},
    "assignees": {"nodes": [{"login": "dave"}]}, "labels": {"nodes": [{"name": "bug", "color": "d73a4a"}]}}]
}}}`))

	page, err := client.SearchIssues("assignee:@me", 10, "CURSOR")
	if err != nil {
		t.Fatal(err)
	}
	variables := server.Requests[0].Variables
	if variables["query"] != "is:issue archived:false assignee:@me sort:updated" || variables["after"] != "CURSOR" {
		t.Errorf("variables = %v", variables)
	}
	want := gh.IssueRow{
		Number: 3, Title: "Crash", Body: "It crashes", URL: "https://github.com/o/r/issues/3", State: "OPEN",
		UpdatedAt: page.Rows[0].UpdatedAt, CreatedAt: page.Rows[0].CreatedAt,
		Author: "carol", Repository: "o/r", CommentCount: 6, ReactionCount: 9,
		Assignees: []string{"dave"}, Labels: []gh.Label{{Name: "bug", Color: "d73a4a"}},
	}
	if page.TotalCount != 1 || page.HasNextPage || page.EndCursor != "END" || !reflect.DeepEqual(page.Rows, []gh.IssueRow{want}) {
		t.Errorf("page = %+v", page)
	}
	if page.Rows[0].UpdatedAt.IsZero() {
		t.Error("updatedAt should be parsed")
	}

	server.GraphQL("... on Issue", ghtest.Reply(500, "boom"))
	if _, err := client.SearchIssues("", 10, ""); err == nil {
		t.Error("a failed search should be an error")
	}
}

func TestNeighbourOfNoStack(t *testing.T) {
	var stack *gh.Stack
	if stack.Neighbour(1, 1) != nil {
		t.Error("a PR outside a stack has no neighbours")
	}
}
