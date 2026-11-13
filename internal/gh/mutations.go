package gh

import (
	"fmt"
	"strconv"
)

func (client *Client) SetFileViewed(pullRequestID, path string, viewed bool) error {
	mutation := "unmarkFileAsViewed"
	if viewed {
		mutation = "markFileAsViewed"
	}
	return client.mutate(fmt.Sprintf(`mutation($id: ID!, $path: String!) { %s(input: { pullRequestId: $id, path: $path }) { clientMutationId } }`, mutation),
		map[string]any{"id": pullRequestID, "path": path}, nil)
}

type NewComment struct {
	Path string
	// "LEFT" for a deleted line, else "RIGHT".
	Side string
	Line int
	// 0 for a single-line comment.
	StartLine int
	Body      string
}

// AddReviewComment adds a comment to the viewer's pending review, starting
// one if needed. Nothing is visible to others until the review is submitted.
func (client *Client) AddReviewComment(pullRequestID string, comment NewComment) error {
	variables := map[string]any{
		"id": pullRequestID, "path": comment.Path, "side": comment.Side, "line": comment.Line, "body": comment.Body,
	}
	if comment.StartLine > 0 && comment.StartLine != comment.Line {
		variables["startLine"] = comment.StartLine
		variables["startSide"] = comment.Side
	}
	var response struct {
		AddPullRequestReviewThread struct{ Thread *struct{ ID string } }
	}
	err := client.mutate(`mutation($id: ID!, $path: String!, $side: DiffSide!, $line: Int!, $body: String!, $startLine: Int, $startSide: DiffSide) {
  addPullRequestReviewThread(input: { pullRequestId: $id, path: $path, side: $side, line: $line, body: $body, startLine: $startLine, startSide: $startSide }) { thread { id } }
}`, variables, &response)
	if err != nil {
		return err
	}
	// GitHub answers a line outside the diff with a null thread, not an error.
	if response.AddPullRequestReviewThread.Thread == nil {
		return fmt.Errorf("GitHub did not accept a comment at %s:%d", comment.Path, comment.Line)
	}
	return nil
}

func (client *Client) ReplyToThread(threadID, body string) error {
	return client.mutate(`mutation($id: ID!, $body: String!) {
  addPullRequestReviewThreadReply(input: { pullRequestReviewThreadId: $id, body: $body }) { comment { id } }
}`, map[string]any{"id": threadID, "body": body}, nil)
}

func (client *Client) SetThreadResolved(threadID string, resolved bool) error {
	mutation := "unresolveReviewThread"
	if resolved {
		mutation = "resolveReviewThread"
	}
	return client.mutate(fmt.Sprintf(`mutation($id: ID!) { %s(input: { threadId: $id }) { thread { id } } }`, mutation),
		map[string]any{"id": threadID}, nil)
}

// SubmitReview publishes the pending review when there is one, and otherwise
// creates a review with just the verdict. Event is APPROVE, REQUEST_CHANGES or COMMENT.
func (client *Client) SubmitReview(pullRequestID, pendingReviewID, event, body string) error {
	if pendingReviewID != "" {
		return client.mutate(`mutation($id: ID!, $event: PullRequestReviewEvent!, $body: String) {
  submitPullRequestReview(input: { pullRequestReviewId: $id, event: $event, body: $body }) { pullRequestReview { id } }
}`, map[string]any{"id": pendingReviewID, "event": event, "body": body}, nil)
	}
	return client.mutate(`mutation($id: ID!, $event: PullRequestReviewEvent!, $body: String) {
  addPullRequestReview(input: { pullRequestId: $id, event: $event, body: $body }) { pullRequestReview { id } }
}`, map[string]any{"id": pullRequestID, "event": event, "body": body}, nil)
}

func (client *Client) DiscardPendingReview(pendingReviewID string) error {
	return client.mutate(`mutation($id: ID!) { deletePullRequestReview(input: { pullRequestReviewId: $id }) { pullRequestReview { id } } }`,
		map[string]any{"id": pendingReviewID}, nil)
}

// Comment posts a conversation comment on a PR or an issue.
func (client *Client) Comment(repository string, number int, body string) error {
	return client.post(fmt.Sprintf("repos/%s/issues/%d/comments", repository, number), map[string]string{"body": body}, nil)
}

// Merge merges one PR, or with stackMerge everything in its stack up to and
// including it, which only `gh stack merge` can do atomically. Method is a gh
// flag (--squash, --merge, --rebase), "--auto" or "" to let a merge queue decide.
func (client *Client) Merge(repository string, number int, method string, stackMerge bool) error {
	arguments := []string{"pr", "merge", strconv.Itoa(number)}
	if stackMerge {
		arguments = []string{"stack", "merge", strconv.Itoa(number), "--yes"}
	}
	switch method {
	case "":
	case "--auto":
		arguments = append(arguments, "--squash", "--auto")
	default:
		arguments = append(arguments, method)
	}
	return client.runGh(repository, "", arguments...)
}

// PullRequestCommand runs `gh pr <arguments>` for the list actions gh-dash
// also delegates to gh: close, reopen, ready, update-branch, edit, checkout.
func (client *Client) PullRequestCommand(repository, directory string, arguments ...string) error {
	return client.runGh(repository, directory, append([]string{"pr"}, arguments...)...)
}

func (client *Client) IssueCommand(repository string, arguments ...string) error {
	return client.runGh(repository, "", append([]string{"issue"}, arguments...)...)
}
