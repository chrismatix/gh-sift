package gh

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

type ThreadComment struct {
	ID         string
	DatabaseID int
	Author     string
	Body       string
	CreatedAt  time.Time
	// True while the comment sits in the viewer's unsubmitted review.
	Pending bool
}

type Thread struct {
	ID         string
	IsResolved bool
	ResolvedBy string
	IsOutdated bool
	Path       string
	// Line and StartLine are 0 when the thread is outdated: GitHub can no
	// longer place it in the current diff.
	Line      int
	StartLine int
	// "LEFT" for a comment on a deleted line, else "RIGHT".
	Side         string
	OriginalLine int
	DiffHunk     string
	Comments     []ThreadComment
}

type Check struct {
	Name        string
	Workflow    string
	Status      string
	Conclusion  string
	StartedAt   time.Time
	CompletedAt time.Time
	// The workflow run and what triggered it; zero and empty for checks that
	// are not GitHub Actions jobs (commit statuses, third-party apps).
	RunID int
	Event string
	JobID int
	URL   string
	// What a third-party check reports instead of a log, as markdown.
	Summary string
}

func (check Check) Running() bool { return check.Status != "COMPLETED" }

// IsJob reports whether the check is an Actions job, with steps and a log.
func (check Check) IsJob() bool { return check.RunID != 0 }

func (check Check) Skipped() bool {
	return check.Conclusion == "SKIPPED" || check.Conclusion == "NEUTRAL" || check.Conclusion == "CANCELLED"
}

func (check Check) Failed() bool {
	if check.Running() {
		return false
	}
	switch check.Conclusion {
	case "SUCCESS", "NEUTRAL", "SKIPPED", "CANCELLED":
		return false
	}
	return true
}

type TimelineItem struct {
	// description, comment, review, force-push, commit, merged, review-requested, closed, reopened
	Kind      string
	Author    string
	Body      string
	State     string
	CreatedAt time.Time
}

type FileSummary struct {
	Path      string
	Additions int
	Deletions int
	// VIEWED, UNVIEWED, or DISMISSED (viewed, then changed).
	ViewedState string
}

type Review struct {
	Author string
	State  string
}

type PullRequest struct {
	PullRequestRow
	Body              string
	HeadRefOid        string
	ChangedFiles      int
	AutoMergeMethod   string
	MergeQueueEnabled bool
	ReviewRequests    []string
	LatestReviews     []Review
	// The viewer's unsubmitted review, if they have started one.
	PendingReviewID string
	Threads         []Thread
	Files           []FileSummary
	Checks          []Check
	Timeline        []TimelineItem
	// Merge methods the repository allows, as gh flags: --squash, --merge, --rebase.
	MergeMethods []string
}

func (pullRequest *PullRequest) UnresolvedThreads() int {
	count := 0
	for _, thread := range pullRequest.Threads {
		if !thread.IsResolved {
			count++
		}
	}
	return count
}

func (pullRequest *PullRequest) PendingComments() int {
	count := 0
	for _, thread := range pullRequest.Threads {
		for _, comment := range thread.Comments {
			if comment.Pending {
				count++
			}
		}
	}
	return count
}

type rawCheck struct {
	Typename    string `json:"__typename"`
	DatabaseID  int
	Name        string
	Status      string
	Conclusion  string
	StartedAt   time.Time
	CompletedAt time.Time
	DetailsURL  string
	Title       string
	Summary     string
	CheckSuite  struct {
		WorkflowRun *struct {
			DatabaseID int
			Event      string
			Workflow   struct{ Name string }
		}
	}
	// StatusContext fields.
	Context   string
	State     string
	CreatedAt time.Time
	TargetURL string
}

func (raw rawCheck) convert() Check {
	if raw.Typename == "StatusContext" {
		check := Check{Name: raw.Context, Status: "COMPLETED", Conclusion: raw.State, StartedAt: raw.CreatedAt, URL: raw.TargetURL}
		if raw.State == "PENDING" || raw.State == "EXPECTED" {
			check.Status, check.Conclusion = "IN_PROGRESS", ""
		}
		return check
	}
	check := Check{
		Name: raw.Name, Status: raw.Status, Conclusion: raw.Conclusion,
		StartedAt: raw.StartedAt, CompletedAt: raw.CompletedAt, JobID: raw.DatabaseID, URL: raw.DetailsURL,
	}
	if run := raw.CheckSuite.WorkflowRun; run != nil {
		check.RunID, check.Event, check.Workflow = run.DatabaseID, run.Event, run.Workflow.Name
	} else {
		check.Summary = strings.TrimSpace(raw.Title + "\n\n" + raw.Summary)
	}
	return check
}

var pullRequestQuery = `
query($owner: String!, $name: String!, $number: Int!) {
  repository(owner: $owner, name: $name) {
    mergeCommitAllowed squashMergeAllowed rebaseMergeAllowed
    pullRequest(number: $number) {
      id number title body url state isDraft isInMergeQueue isMergeQueueEnabled
      mergeable mergeStateStatus reviewDecision
      additions deletions changedFiles headRefName baseRefName headRefOid updatedAt createdAt
      author { login } authorAssociation
      repository { nameWithOwner }
      autoMergeRequest { mergeMethod }
      comments { totalCount }
      assignees(first: 10) { nodes { login } }
      labels(first: 20) { nodes { name color } }
      reviewRequests(first: 10) { nodes { requestedReviewer { ... on User { login } ... on Team { name } ... on Bot { login } } } }
      latestReviews(first: 20) { nodes { author { login } state } }
      pendingReviews: reviews(states: PENDING, first: 1) { nodes { id } }
      ` + stackFieldsForDetail + `
      reviewThreads(first: 100) {
        nodes {
          id isResolved resolvedBy { login } isOutdated path line startLine originalLine diffSide
          hunk: comments(first: 1) { nodes { diffHunk } }
          comments(first: 50) { nodes { id databaseId author { login } body createdAt state } }
        }
      }
      files(first: 100) { pageInfo { hasNextPage endCursor } nodes { path additions deletions viewerViewedState } }
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              state
              contexts(first: 100) {
                checkRunCountsByState { state count }
                statusContextCountsByState { state count }
                nodes {
                  __typename
                  ... on CheckRun {
                    databaseId name status conclusion startedAt completedAt detailsUrl title summary
                    checkSuite { workflowRun { databaseId event workflow { name } } }
                  }
                  ... on StatusContext { context state createdAt targetUrl }
                }
              }
            }
          }
        }
      }
      timelineItems(last: 80, itemTypes: [ISSUE_COMMENT, PULL_REQUEST_REVIEW, HEAD_REF_FORCE_PUSHED_EVENT, PULL_REQUEST_COMMIT, MERGED_EVENT, REVIEW_REQUESTED_EVENT, CLOSED_EVENT, REOPENED_EVENT]) {
        nodes {
          __typename
          ... on IssueComment { author { login } body createdAt }
          ... on PullRequestReview { author { login } body state createdAt }
          ... on HeadRefForcePushedEvent { actor { login } createdAt }
          ... on PullRequestCommit { commit { abbreviatedOid messageHeadline committedDate } }
          ... on MergedEvent { actor { login } createdAt }
          ... on ReviewRequestedEvent { actor { login } createdAt requestedReviewer { ... on User { login } ... on Team { name } ... on Bot { login } } }
          ... on ClosedEvent { actor { login } createdAt }
          ... on ReopenedEvent { actor { login } createdAt }
        }
      }
    }
  }
}`

type rawReviewer struct {
	Login string
	Name  string
}

func (reviewer rawReviewer) name() string {
	if reviewer.Login != "" {
		return reviewer.Login
	}
	return reviewer.Name
}

type rawFiles = nodeList[struct {
	Path              string
	Additions         int
	Deletions         int
	ViewerViewedState string
}]

type rawPullRequest struct {
	rawPullRequestRow
	Body                string
	HeadRefOid          string
	ChangedFiles        int
	IsMergeQueueEnabled bool
	AutoMergeRequest    *struct{ MergeMethod string }
	ReviewRequests      nodeList[struct{ RequestedReviewer rawReviewer }]
	LatestReviews       nodeList[struct {
		Author *actor
		State  string
	}]
	PendingReviews nodeList[struct{ ID string }]
	ReviewThreads  nodeList[struct {
		ID           string
		IsResolved   bool
		ResolvedBy   *actor
		IsOutdated   bool
		Path         string
		Line         int
		StartLine    int
		OriginalLine int
		DiffSide     string
		Hunk         nodeList[struct{ DiffHunk string }]
		Comments     nodeList[struct {
			ID         string
			DatabaseID int
			Author     *actor
			Body       string
			CreatedAt  time.Time
			State      string
		}]
	}]
	Files         rawFiles
	TimelineItems nodeList[struct {
		Typename  string `json:"__typename"`
		Author    *actor
		Actor     *actor
		Body      string
		State     string
		CreatedAt time.Time
		Commit    struct {
			AbbreviatedOid  string
			MessageHeadline string
			CommittedDate   time.Time
		}
		RequestedReviewer rawReviewer
	}]
}

func (client *Client) PullRequest(repository string, number int) (*PullRequest, error) {
	owner, name, _ := strings.Cut(repository, "/")
	var response struct {
		Repository struct {
			MergeCommitAllowed bool
			SquashMergeAllowed bool
			RebaseMergeAllowed bool
			PullRequest        *rawPullRequest
		}
	}
	variables := map[string]any{"owner": owner, "name": name, "number": number}
	if err := client.query(pullRequestQuery, variables, &response); err != nil {
		return nil, err
	}
	raw := response.Repository.PullRequest
	if raw == nil {
		return nil, fmt.Errorf("%s#%d not found", repository, number)
	}

	pullRequest := &PullRequest{
		PullRequestRow:    raw.rawPullRequestRow.convert(),
		Body:              raw.Body,
		HeadRefOid:        raw.HeadRefOid,
		ChangedFiles:      raw.ChangedFiles,
		MergeQueueEnabled: raw.IsMergeQueueEnabled,
	}
	pullRequest.CommentCount = raw.Comments.TotalCount
	if raw.AutoMergeRequest != nil {
		pullRequest.AutoMergeMethod = raw.AutoMergeRequest.MergeMethod
	}
	if len(raw.PendingReviews.Nodes) > 0 {
		pullRequest.PendingReviewID = raw.PendingReviews.Nodes[0].ID
	}
	if response.Repository.SquashMergeAllowed {
		pullRequest.MergeMethods = append(pullRequest.MergeMethods, "--squash")
	}
	if response.Repository.MergeCommitAllowed {
		pullRequest.MergeMethods = append(pullRequest.MergeMethods, "--merge")
	}
	if response.Repository.RebaseMergeAllowed {
		pullRequest.MergeMethods = append(pullRequest.MergeMethods, "--rebase")
	}
	for _, request := range raw.ReviewRequests.Nodes {
		pullRequest.ReviewRequests = append(pullRequest.ReviewRequests, request.RequestedReviewer.name())
	}
	for _, review := range raw.LatestReviews.Nodes {
		pullRequest.LatestReviews = append(pullRequest.LatestReviews, Review{Author: review.Author.login(), State: review.State})
	}

	for _, rawThread := range raw.ReviewThreads.Nodes {
		thread := Thread{
			ID: rawThread.ID, IsResolved: rawThread.IsResolved, IsOutdated: rawThread.IsOutdated,
			Path: rawThread.Path, Line: rawThread.Line, StartLine: rawThread.StartLine,
			OriginalLine: rawThread.OriginalLine, Side: rawThread.DiffSide,
		}
		if rawThread.ResolvedBy != nil {
			thread.ResolvedBy = rawThread.ResolvedBy.login()
		}
		if len(rawThread.Hunk.Nodes) > 0 {
			thread.DiffHunk = rawThread.Hunk.Nodes[0].DiffHunk
		}
		for _, comment := range rawThread.Comments.Nodes {
			thread.Comments = append(thread.Comments, ThreadComment{
				ID: comment.ID, DatabaseID: comment.DatabaseID, Author: comment.Author.login(),
				Body: comment.Body, CreatedAt: comment.CreatedAt, Pending: comment.State == "PENDING",
			})
		}
		pullRequest.Threads = append(pullRequest.Threads, thread)
	}

	if len(raw.Commits.Nodes) > 0 && raw.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		for _, check := range raw.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes {
			pullRequest.Checks = append(pullRequest.Checks, check.convert())
		}
	}

	for _, item := range raw.TimelineItems.Nodes {
		switch item.Typename {
		case "IssueComment":
			pullRequest.Timeline = append(pullRequest.Timeline, TimelineItem{Kind: "comment", Author: item.Author.login(), Body: item.Body, CreatedAt: item.CreatedAt})
		case "PullRequestReview":
			// A pending review is the viewer's own draft, not an event.
			if item.State != "PENDING" {
				pullRequest.Timeline = append(pullRequest.Timeline, TimelineItem{Kind: "review", Author: item.Author.login(), Body: item.Body, State: item.State, CreatedAt: item.CreatedAt})
			}
		case "HeadRefForcePushedEvent":
			pullRequest.Timeline = append(pullRequest.Timeline, TimelineItem{Kind: "force-push", Author: item.Actor.login(), CreatedAt: item.CreatedAt})
		case "PullRequestCommit":
			pullRequest.Timeline = append(pullRequest.Timeline, TimelineItem{Kind: "commit", Author: item.Commit.AbbreviatedOid, Body: item.Commit.MessageHeadline, CreatedAt: item.Commit.CommittedDate})
		case "MergedEvent":
			pullRequest.Timeline = append(pullRequest.Timeline, TimelineItem{Kind: "merged", Author: item.Actor.login(), CreatedAt: item.CreatedAt})
		case "ReviewRequestedEvent":
			pullRequest.Timeline = append(pullRequest.Timeline, TimelineItem{Kind: "review-requested", Author: item.Actor.login(), Body: item.RequestedReviewer.name(), CreatedAt: item.CreatedAt})
		case "ClosedEvent":
			pullRequest.Timeline = append(pullRequest.Timeline, TimelineItem{Kind: "closed", Author: item.Actor.login(), CreatedAt: item.CreatedAt})
		case "ReopenedEvent":
			pullRequest.Timeline = append(pullRequest.Timeline, TimelineItem{Kind: "reopened", Author: item.Actor.login(), CreatedAt: item.CreatedAt})
		}
	}

	files := raw.Files
	for {
		for _, file := range files.Nodes {
			pullRequest.Files = append(pullRequest.Files, FileSummary{Path: file.Path, Additions: file.Additions, Deletions: file.Deletions, ViewedState: file.ViewerViewedState})
		}
		if !files.PageInfo.HasNextPage {
			break
		}
		var err error
		if files, err = client.moreFiles(owner, name, number, files.PageInfo.EndCursor); err != nil {
			return nil, err
		}
	}
	return pullRequest, nil
}

func (client *Client) moreFiles(owner, name string, number int, after string) (rawFiles, error) {
	var response struct {
		Repository struct{ PullRequest struct{ Files rawFiles } }
	}
	err := client.query(`
query($owner: String!, $name: String!, $number: Int!, $after: String!) {
  repository(owner: $owner, name: $name) { pullRequest(number: $number) {
    files(first: 100, after: $after) { pageInfo { hasNextPage endCursor } nodes { path additions deletions viewerViewedState } }
  } }
}`, map[string]any{"owner": owner, "name": name, "number": number, "after": after}, &response)
	return response.Repository.PullRequest.Files, err
}

// Diff returns the PR's unified diff. GitHub refuses to render very large
// diffs in one piece; those are stitched together from the per-file patches.
func (client *Client) Diff(repository string, number int) (string, error) {
	unified, err := client.getText(fmt.Sprintf("repos/%s/pulls/%d", repository, number), "application/vnd.github.diff")
	if err == nil {
		return unified, nil
	}
	if !strings.Contains(err.Error(), "406") && !strings.Contains(err.Error(), "too_large") {
		return "", err
	}
	var builder strings.Builder
	for page := 1; ; page++ {
		var files []struct {
			Filename         string
			PreviousFilename string `json:"previous_filename"`
			Status           string
			Patch            string
		}
		if err := client.rest.Get(fmt.Sprintf("repos/%s/pulls/%d/files?per_page=100&page=%d", repository, number, page), &files); err != nil {
			return "", err
		}
		for _, file := range files {
			oldName := file.Filename
			if file.PreviousFilename != "" {
				oldName = file.PreviousFilename
			}
			fmt.Fprintf(&builder, "diff --git a/%s b/%s\n", oldName, file.Filename)
			switch file.Status {
			case "added":
				builder.WriteString("new file mode 100644\n")
			case "removed":
				builder.WriteString("deleted file mode 100644\n")
			case "renamed":
				fmt.Fprintf(&builder, "rename from %s\nrename to %s\n", oldName, file.Filename)
			}
			if file.Patch == "" && file.Status != "renamed" {
				// GitHub omits the patch for binary and oversized files.
				fmt.Fprintf(&builder, "Binary files a/%s and b/%s differ\n", oldName, file.Filename)
			}
			builder.WriteString(file.Patch)
			builder.WriteString("\n")
		}
		if len(files) < 100 {
			return builder.String(), nil
		}
	}
}

// PullRequestForBranch finds the PR whose head is the branch: the open one
// if there is one, else the most recent, as `gh pr view <branch>` does.
func (client *Client) PullRequestForBranch(repository, branch string) (int, error) {
	owner, name, _ := strings.Cut(repository, "/")
	var response struct {
		Repository struct {
			PullRequests nodeList[struct {
				Number int
				State  string
			}]
		}
	}
	err := client.query(`
query($owner: String!, $name: String!, $branch: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(headRefName: $branch, first: 20, orderBy: { field: CREATED_AT, direction: DESC }) { nodes { number state } }
  }
}`, map[string]any{"owner": owner, "name": name, "branch": branch}, &response)
	if err != nil {
		return 0, err
	}
	found := response.Repository.PullRequests.Nodes
	if len(found) == 0 {
		return 0, fmt.Errorf("no pull request in %s for branch %s", repository, branch)
	}
	for _, pullRequest := range found {
		if pullRequest.State == "OPEN" {
			return pullRequest.Number, nil
		}
	}
	return found[0].Number, nil
}

// Attributes returns the .gitattributes at the root of ref, or "" when there is none.
func (client *Client) Attributes(repository, ref string) (string, error) {
	text, err := client.getText(fmt.Sprintf("repos/%s/contents/.gitattributes?ref=%s", repository, ref), "application/vnd.github.raw")
	var httpError *api.HTTPError
	if errors.As(err, &httpError) && httpError.StatusCode == 404 {
		return "", nil
	}
	return text, err
}
