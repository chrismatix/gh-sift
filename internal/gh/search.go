package gh

import (
	"fmt"
	"strings"
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

// PositionOf returns where the PR sits in the stack, or 0 when it is not in it.
func (stack *Stack) PositionOf(number int) int {
	for _, entry := range stack.Entries {
		if entry.Number == number {
			return entry.Position
		}
	}
	return 0
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
	// Set only on rows listed from a merge queue.
	Queue *QueueEntry
}

type QueueEntry struct {
	Position int
	// QUEUED, AWAITING_CHECKS, MERGEABLE, UNMERGEABLE or LOCKED.
	State      string
	EnqueuedAt time.Time
	// Zero when GitHub has no estimate.
	EstimatedTimeToMerge time.Duration
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
	stackFieldsForDetail = fmt.Sprintf(stackFields, "commits(last: 1) { nodes { commit { statusCheckRollup { state contexts(first: 0) { checkRunCountsByState { state count } statusContextCountsByState { state count } } } } } }")
)

var pullRequestRowFields = `
        id number title url state isDraft isInMergeQueue mergeable reviewDecision
        additions deletions headRefName baseRefName updatedAt createdAt
        author { login } authorAssociation
        repository { nameWithOwner }
        comments { totalCount } reviewThreads { totalCount } reviews { totalCount }
        assignees(first: 3) { nodes { login } }
        labels(first: 6) { nodes { name color } }
        commits(last: 1) { nodes { commit { statusCheckRollup { state contexts(first: 0) { checkRunCountsByState { state count } statusContextCountsByState { state count } } } } } }
        ` + stackFieldsForList

var searchPullRequestsQuery = `
query($query: String!, $limit: Int!, $after: String) {
  search(query: $query, type: ISSUE, first: $limit, after: $after) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes {
      ... on PullRequest {` + pullRequestRowFields + `
      }
    }
  }
}`

var mergeQueueQuery = `
query($owner: String!, $name: String!, $branch: String, $limit: Int!, $after: String) {
  repository(owner: $owner, name: $name) {
    mergeQueue(branch: $branch) {
      entries(first: $limit, after: $after) {
        totalCount
        pageInfo { hasNextPage endCursor }
        nodes {
          position state enqueuedAt estimatedTimeToMerge
          pullRequest {` + pullRequestRowFields + `
          }
        }
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

// MergeQueue lists the PRs queued to merge into a branch, first in line first.
// An empty branch means the repository's default branch.
func (client *Client) MergeQueue(repository, branch string, limit int, after string) (Page[PullRequestRow], error) {
	owner, name, _ := strings.Cut(repository, "/")
	var response struct {
		Repository struct {
			MergeQueue *struct {
				Entries struct {
					TotalCount int
					PageInfo   struct {
						HasNextPage bool
						EndCursor   string
					}
					Nodes []struct {
						Position             int
						State                string
						EnqueuedAt           time.Time
						EstimatedTimeToMerge int
						PullRequest          rawPullRequestRow
					}
				}
			}
		}
	}
	variables := map[string]any{"owner": owner, "name": name, "limit": limit, "branch": nil, "after": nil}
	if branch != "" {
		variables["branch"] = branch
	}
	if after != "" {
		variables["after"] = after
	}
	if err := client.query(mergeQueueQuery, variables, &response); err != nil {
		return Page[PullRequestRow]{}, err
	}
	queue := response.Repository.MergeQueue
	if queue == nil {
		return Page[PullRequestRow]{}, fmt.Errorf("%s has no merge queue on %s", repository, map[bool]string{true: "its default branch", false: branch}[branch == ""])
	}
	page := Page[PullRequestRow]{TotalCount: queue.Entries.TotalCount, HasNextPage: queue.Entries.PageInfo.HasNextPage, EndCursor: queue.Entries.PageInfo.EndCursor}
	for _, node := range queue.Entries.Nodes {
		row := node.PullRequest.convert()
		row.Queue = &QueueEntry{Position: node.Position, State: node.State, EnqueuedAt: node.EnqueuedAt, EstimatedTimeToMerge: time.Duration(node.EstimatedTimeToMerge) * time.Second}
		page.Rows = append(page.Rows, row)
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
