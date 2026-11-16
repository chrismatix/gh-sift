package gh_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

func TestGraphQLMutations(t *testing.T) {
	cases := []struct {
		name      string
		call      func(client *gh.Client) error
		mutation  string
		variables map[string]any
	}{
		{"mark viewed", func(client *gh.Client) error { return client.SetFileViewed("PR_1", "main.go", true) },
			"markFileAsViewed(", map[string]any{"id": "PR_1", "path": "main.go"}},
		{"unmark viewed", func(client *gh.Client) error { return client.SetFileViewed("PR_1", "main.go", false) },
			"unmarkFileAsViewed(", map[string]any{"id": "PR_1", "path": "main.go"}},
		{"reply", func(client *gh.Client) error { return client.ReplyToThread("T_1", "Done") },
			"addPullRequestReviewThreadReply(", map[string]any{"id": "T_1", "body": "Done"}},
		{"resolve", func(client *gh.Client) error { return client.SetThreadResolved("T_1", true) },
			" resolveReviewThread(", map[string]any{"id": "T_1"}},
		{"unresolve", func(client *gh.Client) error { return client.SetThreadResolved("T_1", false) },
			"unresolveReviewThread(", map[string]any{"id": "T_1"}},
		{"submit pending", func(client *gh.Client) error { return client.SubmitReview("PR_1", "PRR_1", "APPROVE", "Ship it") },
			"submitPullRequestReview(", map[string]any{"id": "PRR_1", "event": "APPROVE", "body": "Ship it"}},
		{"new review", func(client *gh.Client) error { return client.SubmitReview("PR_1", "", "REQUEST_CHANGES", "No") },
			"addPullRequestReview(", map[string]any{"id": "PR_1", "event": "REQUEST_CHANGES", "body": "No"}},
		{"discard", func(client *gh.Client) error { return client.DiscardPendingReview("PRR_1") },
			"deletePullRequestReview(", map[string]any{"id": "PRR_1"}},
		{"single-line comment", func(client *gh.Client) error {
			return client.AddReviewComment("PR_1", gh.NewComment{Path: "main.go", Side: "RIGHT", Line: 4, StartLine: 4, Body: "Why?"})
		}, "addPullRequestReviewThread(", map[string]any{"id": "PR_1", "path": "main.go", "side": "RIGHT", "line": float64(4), "body": "Why?"}},
		{"multi-line comment", func(client *gh.Client) error {
			return client.AddReviewComment("PR_1", gh.NewComment{Path: "main.go", Side: "LEFT", Line: 6, StartLine: 2, Body: "Range"})
		}, "addPullRequestReviewThread(", map[string]any{
			"id": "PR_1", "path": "main.go", "side": "LEFT", "line": float64(6), "body": "Range", "startLine": float64(2), "startSide": "LEFT",
		}},
	}
	for _, each := range cases {
		t.Run(each.name, func(t *testing.T) {
			server, client := ghtest.New(t)
			server.GraphQL("mutation", ghtest.Reply(200, map[string]any{
				"addPullRequestReviewThread": map[string]any{"thread": map[string]any{"id": "T_new"}},
			}))
			if err := each.call(client); err != nil {
				t.Fatal(err)
			}
			sent := server.Find(each.mutation)
			if len(sent) != 1 {
				t.Fatalf("expected one %s, sent %+v", each.mutation, server.Requests)
			}
			if !reflect.DeepEqual(sent[0].Variables, each.variables) {
				t.Errorf("variables = %v, want %v", sent[0].Variables, each.variables)
			}

			client.ReadOnly = true
			if err := each.call(client); !errors.Is(err, gh.ErrReadOnly) {
				t.Errorf("read-only error = %v", err)
			}
			if len(server.Requests) != 1 {
				t.Error("a read-only client must not send the mutation")
			}
		})
	}
}

func TestAddReviewCommentOutsideTheDiff(t *testing.T) {
	server, client := ghtest.New(t)
	server.GraphQL("addPullRequestReviewThread", ghtest.Reply(200, map[string]any{"addPullRequestReviewThread": map[string]any{"thread": nil}}))
	err := client.AddReviewComment("PR_1", gh.NewComment{Path: "main.go", Side: "RIGHT", Line: 99, Body: "?"})
	if err == nil || err.Error() != "GitHub did not accept a comment at main.go:99" {
		t.Errorf("err = %v", err)
	}

	server.GraphQL("addPullRequestReviewThread", ghtest.Reply(500, "boom"))
	if err := client.AddReviewComment("PR_1", gh.NewComment{Path: "main.go", Side: "RIGHT", Line: 1, Body: "?"}); err == nil {
		t.Error("a failed mutation should be an error")
	}
}

func TestRESTWrites(t *testing.T) {
	server, client := ghtest.New(t)
	server.REST("POST", "repos/o/r/issues/7/comments", ghtest.Reply(201, map[string]any{"id": 1}))
	server.REST("POST", "repos/o/r/actions/runs/5/rerun-failed-jobs", ghtest.Reply(201, map[string]any{}))

	if err := client.Comment("o/r", 7, "Thanks!"); err != nil {
		t.Fatal(err)
	}
	if err := client.RerunFailedJobs("o/r", 5); err != nil {
		t.Fatal(err)
	}
	if body := server.Find("issues/7/comments")[0].Body; body != `{"body":"Thanks!"}` {
		t.Errorf("comment body = %s", body)
	}
	if len(server.Find("rerun-failed-jobs")) != 1 {
		t.Errorf("requests = %+v", server.Requests)
	}

	client.ReadOnly = true
	if err := client.Comment("o/r", 7, "x"); !errors.Is(err, gh.ErrReadOnly) {
		t.Errorf("read-only comment = %v", err)
	}
	if err := client.RerunFailedJobs("o/r", 5); !errors.Is(err, gh.ErrReadOnly) {
		t.Errorf("read-only rerun = %v", err)
	}
	if len(server.Requests) != 2 {
		t.Error("a read-only client must not post")
	}

	client.ReadOnly = false
	if err := client.Comment("o/r", 8, "x"); err == nil {
		t.Error("an unanswered post should be an error")
	}
}

// ghFakeGh puts a gh on PATH that records its arguments and environment,
// and fails with output when its last argument is "fail", silently when it is "silent".
func ghFakeGh(t *testing.T) (logPath string) {
	directory := t.TempDir()
	logPath = filepath.Join(directory, "calls")
	script := `#!/bin/sh
echo "$PWD|$GH_REPO|$GH_PROMPT_DISABLED|$*" >> "` + logPath + `"
for last; do :; done
case "$last" in
fail) echo "merge conflict" >&2; exit 1 ;;
silent) exit 3 ;;
esac
`
	if err := os.WriteFile(filepath.Join(directory, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestGhCommands(t *testing.T) {
	logPath := ghFakeGh(t)
	_, client := ghtest.New(t)
	directory, _ := filepath.EvalSymlinks(t.TempDir())
	here, _ := os.Getwd()

	calls := []func() error{
		func() error { return client.Merge("o/r", 7, "", false) },
		func() error { return client.Merge("o/r", 7, "--auto", false) },
		func() error { return client.Merge("o/r", 7, "--rebase", true) },
		func() error { return client.PullRequestCommand("o/r", directory, "checkout", "7") },
		func() error { return client.IssueCommand("o/r", "close", "3") },
	}
	for _, call := range calls {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	contents, _ := os.ReadFile(logPath)
	want := []string{
		here + "|o/r|1|pr merge 7",
		here + "|o/r|1|pr merge 7 --squash --auto",
		here + "|o/r|1|stack merge 7 --yes --rebase",
		directory + "|o/r|1|pr checkout 7",
		here + "|o/r|1|issue close 3",
	}
	if got := strings.Split(strings.TrimSpace(string(contents)), "\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("gh calls =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	if err := client.PullRequestCommand("o/r", "", "x"); err != nil {
		t.Fatal(err)
	}
	if err := client.IssueCommand("o/r"); err != nil {
		t.Fatal(err)
	}
}

func TestGhCommandFailures(t *testing.T) {
	ghFakeGh(t)
	_, client := ghtest.New(t)
	if err := client.PullRequestCommand("o/r", "", "close", "fail"); err == nil || err.Error() != "merge conflict" {
		t.Errorf("gh's own message should be the error, got %v", err)
	}
	if err := client.PullRequestCommand("o/r", "", "close", "silent"); err == nil || err.Error() != "gh pr: exit status 3" {
		t.Errorf("silent failure = %v", err)
	}

	client.ReadOnly = true
	if err := client.Merge("o/r", 7, "", false); !errors.Is(err, gh.ErrReadOnly) {
		t.Errorf("read-only merge = %v", err)
	}
}

func TestNewClient(t *testing.T) {
	t.Setenv("GH_TOKEN", "test")
	t.Setenv("SIFT_READ_ONLY", "1")
	client, err := gh.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	if !client.ReadOnly {
		t.Error("SIFT_READ_ONLY should make the client read-only")
	}

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	// go-gh falls back to `gh auth token`, which would find the keyring's token.
	t.Setenv("PATH", t.TempDir())
	if _, err := gh.NewClientWithOptions(api.ClientOptions{Host: "github.com"}); err == nil {
		t.Error("a client without a token should fail to build")
	}
}
