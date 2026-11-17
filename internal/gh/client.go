// Package gh talks to GitHub with the credentials of the gh CLI.
package gh

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
)

type Client struct {
	graphql *api.GraphQLClient
	rest    *api.RESTClient
	options api.ClientOptions
	retry   *retryTransport
	// Set by SIFT_READ_ONLY=1: every write is refused, so the app can be
	// tried against repositories that must not be touched.
	ReadOnly bool
}

func NewClient() (*Client, error) {
	return NewClientWithOptions(api.ClientOptions{Timeout: 60 * time.Second})
}

// NewClientWithOptions lets tests route every request to a fake GitHub through options.Transport.
func NewClientWithOptions(options api.ClientOptions) (*Client, error) {
	retry := &retryTransport{next: options.Transport, baseDelay: time.Second}
	if retry.next == nil {
		retry.next = http.DefaultTransport
	}
	options.Transport = retry
	graphql, err := api.NewGraphQLClient(options)
	if err != nil {
		return nil, err
	}
	rest, err := api.NewRESTClient(options)
	if err != nil {
		return nil, err
	}
	return &Client{graphql: graphql, rest: rest, options: options, retry: retry, ReadOnly: os.Getenv("SIFT_READ_ONLY") != ""}, nil
}

var ErrReadOnly = errors.New("read-only mode (SIFT_READ_ONLY is set)")

func (client *Client) query(query string, variables map[string]any, response any) error {
	return client.graphql.Do(query, variables, response)
}

func (client *Client) mutate(query string, variables map[string]any, response any) error {
	if client.ReadOnly {
		return ErrReadOnly
	}
	if response == nil {
		response = &struct{}{}
	}
	return client.graphql.Do(query, variables, response)
}

// getText fetches a REST path whose body is not JSON, such as a diff or a log.
func (client *Client) getText(path, accept string) (string, error) {
	rest := client.rest
	if accept != "" {
		options := client.options
		options.Headers = map[string]string{"Accept": accept}
		var err error
		rest, err = api.NewRESTClient(options)
		if err != nil {
			return "", err
		}
	}
	response, err := rest.Request("GET", path, nil)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	return string(body), err
}

func (client *Client) post(path string, payload, response any) error {
	if client.ReadOnly {
		return ErrReadOnly
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return client.rest.Post(path, bytes.NewReader(body), response)
}

// runGh runs a gh subcommand for the writes that gh already does well
// (merging, stack merging, checkout).
func (client *Client) runGh(repository, directory string, arguments ...string) error {
	if client.ReadOnly {
		return ErrReadOnly
	}
	command := exec.Command("gh", arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GH_REPO="+repository, "GH_PROMPT_DISABLED=1")
	output, err := command.CombinedOutput()
	if err != nil {
		if text := strings.TrimSpace(string(output)); text != "" {
			return errors.New(text)
		}
		return fmt.Errorf("gh %s: %w", arguments[0], err)
	}
	return nil
}

func (client *Client) ViewerLogin() (string, error) {
	var response struct {
		Viewer struct{ Login string }
	}
	err := client.query(`query { viewer { login } }`, nil, &response)
	return response.Viewer.Login, err
}

type nodeList[T any] struct {
	Nodes      []T
	TotalCount int
	PageInfo   struct {
		HasNextPage bool
		EndCursor   string
	}
}

type actor struct {
	Login string
}

// GitHub returns null for deleted accounts.
func (author *actor) login() string {
	if author == nil || author.Login == "" {
		return "ghost"
	}
	return strings.TrimSuffix(author.Login, "[bot]")
}

type lastCommit struct {
	Nodes []struct {
		Commit struct {
			StatusCheckRollup *struct {
				State    string
				Contexts struct {
					nodeList[rawCheck]
					CheckRunCountsByState      []stateCount
					StatusContextCountsByState []stateCount
				}
			}
		}
	}
}

type stateCount struct {
	State string
	Count int
}

// checksState sums up the checks the way the Checks tab judges them. GitHub's
// own rollup fails a commit for a cancelled run, even one a rerun superseded.
func (commits lastCommit) checksState() string {
	if len(commits.Nodes) == 0 || commits.Nodes[0].Commit.StatusCheckRollup == nil {
		return ""
	}
	rollup := commits.Nodes[0].Commit.StatusCheckRollup
	failed, pending := false, false
	for _, each := range append(rollup.Contexts.CheckRunCountsByState, rollup.Contexts.StatusContextCountsByState...) {
		if each.Count == 0 {
			continue
		}
		switch each.State {
		case "FAILURE", "ERROR", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE":
			failed = true
		case "PENDING", "EXPECTED", "IN_PROGRESS", "QUEUED", "WAITING":
			pending = true
		}
	}
	switch {
	case failed:
		return "FAILURE"
	case pending:
		return "PENDING"
	case len(rollup.Contexts.CheckRunCountsByState)+len(rollup.Contexts.StatusContextCountsByState) == 0:
		return rollup.State
	}
	return "SUCCESS"
}

// Health reports the rate-limit budget and whether requests are on hold.
func (client *Client) Health() Health {
	return client.retry.Health()
}

// SetRetryDelay sets the first backoff step, so tests need not wait seconds.
func (client *Client) SetRetryDelay(delay time.Duration) {
	client.retry.baseDelay = delay
}
