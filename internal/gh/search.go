package gh

import (
	"fmt"
	"time"
)

type Label struct {
	Name  string
	Color string
}

// Stack is a GitHub stacked-PR chain. Entries are ordered bottom first:
// position 1 sits on the base branch and merges first.
type Stack struct {
	ID          string
	Number      int
	Size        int
	BaseRefName string
	Entries     []StackEntry
}

type StackEntry struct {
	Position       int
	Number         int
	Title          string
	State          string
	IsDraft        bool
	IsInMergeQueue bool
	ReviewDecision string
	ChecksState    string
}

// Neighbour returns the PR `delta` positions above (positive) or below the
// given position, or nil at the end of the stack.
func (stack *Stack) Neighbour(position, delta int) *StackEntry {
	if stack == nil {
		return nil
	}
	for index := range stack.Entries {
		if stack.Entries[index].Position == position+delta {
			return &stack.Entries[index]
		}
	}
	return nil
}

type PullRequestRow struct {
	ID                string
	Number            int
	Title             string
	URL               string
	State             string
	IsDraft           bool
	IsInMergeQueue    bool
	Mergeable         string
	MergeStateStatus  string
	ReviewDecision    string
	Additions         int
	Deletions         int
	HeadRefName       string
	BaseRefName       string
	UpdatedAt         time.Time
	CreatedAt         time.Time
	Author            string
	AuthorAssociation string
	Repository        string
	CommentCount      int
	ReviewCount       int
	Assignees         []string
	Labels            []Label
	ChecksState       string
	// Zero and nil when the PR is not part of a stack.
	StackPosition int
	Stack         *Stack
}

type IssueRow struct {
	Number        int
	Title         string
	Body          string
	URL           string
	State         string
	UpdatedAt     time.Time
	CreatedAt     time.Time
	Author        string
	Repository    string
	CommentCount  int
	ReactionCount int
	Assignees     []string
	Labels        []Label
}

type Page[Row any] struct {
	Rows        []Row
	TotalCount  int
	HasNextPage bool
	EndCursor   string
}

type rawStackEntry struct {
	Position int
	Stack    struct {
		ID          string
		Number      int
		Size        int
		BaseRefName string
		Entries     nodeList[struct {
			Position    int
			PullRequest struct {
				Number         int
				Title          string
				State          string
				IsDraft        bool
				IsInMergeQueue bool
				ReviewDecision string
				Commits        lastCommit
			}
		}]
	}
}

func (raw *rawStackEntry) convert() (int, *Stack) {
	if raw == nil {
		return 0, nil
	}
	stack := &Stack{ID: raw.Stack.ID, Number: raw.Stack.Number, Size: raw.Stack.Size, BaseRefName: raw.Stack.BaseRefName}
	for _, entry := range raw.Stack.Entries.Nodes {
		stack.Entries = append(stack.Entries, StackEntry{
			Position:       entry.Position,
			Number:         entry.PullRequest.Number,
			Title:          entry.PullRequest.Title,
			State:          entry.PullRequest.State,
			IsDraft:        entry.PullRequest.IsDraft,
			IsInMergeQueue: entry.PullRequest.IsInMergeQueue,
			ReviewDecision: entry.PullRequest.ReviewDecision,
			ChecksState:    entry.PullRequest.Commits.checksState(),
		})
	}
	return raw.Position, stack
}

// The list leaves out each entry's check state and the PR's merge state:
// together they make the search several seconds slower.
const stackFields = `stackEntry { position stack { id number size baseRefName entries(first: 30) { nodes { position pullRequest {
  number title state isDraft isInMergeQueue reviewDecision %s
} } } } }`

var (
	stackFieldsForList   = fmt.Sprintf(stackFields, "")
	stackFieldsForDetail = fmt.Sprintf(stackFields, "commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }")
)

var searchPullRequestsQuery = `
query($query: String!, $limit: Int!, $after: String) {
  search(query: $query, type: ISSUE, first: $limit, after: $after) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes {
      ... on PullRequest {
        id number title url state isDraft isInMergeQueue mergeable reviewDecision
        additions deletions headRefName baseRefName updatedAt createdAt
        author { login } authorAssociation
        repository { nameWithOwner }
        comments { totalCount } reviewThreads { totalCount } reviews { totalCount }
        assignees(first: 3) { nodes { login } }
        labels(first: 6) { nodes { name color } }
        commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
        ` + stackFieldsForList + `
      }
    }
  }
}`

type rawPullRequestRow struct {
	ID                string
	Number            int
	Title             string
	URL               string
	State             string
	IsDraft           bool
	IsInMergeQueue    bool
	Mergeable         string
	MergeStateStatus  string
	ReviewDecision    string
	Additions         int
	Deletions         int
	HeadRefName       string
	BaseRefName       string
	UpdatedAt         time.Time
	CreatedAt         time.Time
	Author            *actor
	AuthorAssociation string
	Repository        struct{ NameWithOwner string }
	Comments          nodeList[struct{}]
	ReviewThreads     nodeList[struct{}]
	Reviews           nodeList[struct{}]
	Assignees         nodeList[actor]
	Labels            nodeList[Label]
	Commits           lastCommit
	StackEntry        *rawStackEntry
}

func (raw rawPullRequestRow) convert() PullRequestRow {
	row := PullRequestRow{
		ID: raw.ID, Number: raw.Number, Title: raw.Title, URL: raw.URL, State: raw.State,
		IsDraft: raw.IsDraft, IsInMergeQueue: raw.IsInMergeQueue,
		Mergeable: raw.Mergeable, MergeStateStatus: raw.MergeStateStatus, ReviewDecision: raw.ReviewDecision,
		Additions: raw.Additions, Deletions: raw.Deletions,
		HeadRefName: raw.HeadRefName, BaseRefName: raw.BaseRefName,
		UpdatedAt: raw.UpdatedAt, CreatedAt: raw.CreatedAt,
		Author: raw.Author.login(), AuthorAssociation: raw.AuthorAssociation,
		Repository:   raw.Repository.NameWithOwner,
		CommentCount: raw.Comments.TotalCount + raw.ReviewThreads.TotalCount,
		ReviewCount:  raw.Reviews.TotalCount,
		Labels:       raw.Labels.Nodes,
		ChecksState:  raw.Commits.checksState(),
	}
	for _, assignee := range raw.Assignees.Nodes {
		row.Assignees = append(row.Assignees, assignee.Login)
	}
	row.StackPosition, row.Stack = raw.StackEntry.convert()
	return row
}

// SearchPullRequests wraps the filters the way gh-dash does, so a section
// returns the same rows in both tools.
func (client *Client) SearchPullRequests(filters string, limit int, after string) (Page[PullRequestRow], error) {
	var response struct {
		Search struct {
			IssueCount int
			nodeList[rawPullRequestRow]
		}
	}
	variables := map[string]any{"query": "is:pr archived:false " + filters + " sort:updated", "limit": limit}
	if after != "" {
		variables["after"] = after
	}
	if err := client.query(searchPullRequestsQuery, variables, &response); err != nil {
		return Page[PullRequestRow]{}, err
	}
	page := Page[PullRequestRow]{
		TotalCount:  response.Search.IssueCount,
		HasNextPage: response.Search.PageInfo.HasNextPage,
		EndCursor:   response.Search.PageInfo.EndCursor,
	}
	for _, raw := range response.Search.Nodes {
		page.Rows = append(page.Rows, raw.convert())
	}
	return page, nil
}

const searchIssuesQuery = `
query($query: String!, $limit: Int!, $after: String) {
  search(query: $query, type: ISSUE, first: $limit, after: $after) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes {
      ... on Issue {
        number title body url state updatedAt createdAt
        author { login }
        repository { nameWithOwner }
        comments { totalCount }
        reactions { totalCount }
        assignees(first: 3) { nodes { login } }
        labels(first: 6) { nodes { name color } }
      }
    }
  }
}`

func (client *Client) SearchIssues(filters string, limit int, after string) (Page[IssueRow], error) {
	var response struct {
		Search struct {
			IssueCount int
			nodeList[struct {
				Number     int
				Title      string
				Body       string
				URL        string
				State      string
				UpdatedAt  time.Time
				CreatedAt  time.Time
				Author     *actor
				Repository struct{ NameWithOwner string }
				Comments   nodeList[struct{}]
				Reactions  nodeList[struct{}]
				Assignees  nodeList[actor]
				Labels     nodeList[Label]
			}]
		}
	}
	variables := map[string]any{"query": "is:issue archived:false " + filters + " sort:updated", "limit": limit}
	if after != "" {
		variables["after"] = after
	}
	if err := client.query(searchIssuesQuery, variables, &response); err != nil {
		return Page[IssueRow]{}, err
	}
	page := Page[IssueRow]{
		TotalCount:  response.Search.IssueCount,
		HasNextPage: response.Search.PageInfo.HasNextPage,
		EndCursor:   response.Search.PageInfo.EndCursor,
	}
	for _, raw := range response.Search.Nodes {
		row := IssueRow{
			Number: raw.Number, Title: raw.Title, Body: raw.Body, URL: raw.URL, State: raw.State,
			UpdatedAt: raw.UpdatedAt, CreatedAt: raw.CreatedAt,
			Author: raw.Author.login(), Repository: raw.Repository.NameWithOwner,
			CommentCount: raw.Comments.TotalCount, ReactionCount: raw.Reactions.TotalCount,
			Labels: raw.Labels.Nodes,
		}
		for _, assignee := range raw.Assignees.Nodes {
			row.Assignees = append(row.Assignees, assignee.Login)
		}
		page.Rows = append(page.Rows, row)
	}
	return page, nil
}
