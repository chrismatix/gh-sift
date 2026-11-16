package gh

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// scriptedTransport answers each request with the next scripted reply.
type scriptedTransport struct {
	replies []func() (*http.Response, error)
	bodies  []string
}

func (transport *scriptedTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body, _ := io.ReadAll(request.Body)
	transport.bodies = append(transport.bodies, string(body))
	reply := transport.replies[0]
	if len(transport.replies) > 1 {
		transport.replies = transport.replies[1:]
	}
	return reply()
}

func reply(status int, body string, headers ...string) func() (*http.Response, error) {
	return func() (*http.Response, error) {
		response := &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
		for index := 0; index+1 < len(headers); index += 2 {
			response.Header.Set(headers[index], headers[index+1])
		}
		return response, nil
	}
}

func dropped() (*http.Response, error) { return nil, errors.New("connection reset") }

func send(t *testing.T, transport *retryTransport, method, path, body string) (*http.Response, error) {
	t.Helper()
	request, err := http.NewRequest(method, "https://api.github.com"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return transport.RoundTrip(request)
}

func TestRetryBacksOffUntilGitHubAnswers(t *testing.T) {
	cases := []struct {
		name     string
		method   string
		path     string
		body     string
		replies  []func() (*http.Response, error)
		attempts int
		status   int
	}{
		{"a dropped read", "GET", "/repos/o/r/pulls/1", "", []func() (*http.Response, error){dropped, reply(200, "ok")}, 2, 200},
		{"an overloaded query", "POST", "/graphql", `{"query":"query { viewer { login } }"}`, []func() (*http.Response, error){reply(502, ""), reply(503, ""), reply(200, "{}")}, 3, 200},
		{"a write GitHub refused", "POST", "/graphql", `{"query":"mutation { x }"}`, []func() (*http.Response, error){reply(429, ""), reply(200, "{}")}, 2, 200},
		{"a write that may have landed", "POST", "/graphql", `{"query":"mutation { x }"}`, []func() (*http.Response, error){reply(502, ""), reply(200, "{}")}, 1, 502},
		{"a REST write that may have landed", "POST", "/repos/o/r/issues/1/comments", `{}`, []func() (*http.Response, error){reply(504, ""), reply(201, "{}")}, 1, 504},
		{"a client error", "GET", "/repos/o/r/pulls/1", "", []func() (*http.Response, error){reply(404, "")}, 1, 404},
		{"a secondary limit", "GET", "/repos/o/r/pulls/1", "", []func() (*http.Response, error){reply(403, "", "Retry-After", "0"), reply(200, "")}, 2, 200},
		{"an outage", "GET", "/repos/o/r/pulls/1", "", []func() (*http.Response, error){reply(503, "")}, retryAttempts, 503},
	}
	for _, each := range cases {
		t.Run(each.name, func(t *testing.T) {
			script := &scriptedTransport{replies: each.replies}
			transport := &retryTransport{next: script, baseDelay: time.Millisecond}
			response, err := send(t, transport, each.method, each.path, each.body)
			if len(script.bodies) != each.attempts {
				t.Errorf("attempts = %d, want %d", len(script.bodies), each.attempts)
			}
			for _, body := range script.bodies {
				if body != each.body {
					t.Errorf("a retry sent %q, want the original body %q", body, each.body)
				}
			}
			if err != nil || response.StatusCode != each.status {
				t.Errorf("got %v, %v; want status %d", response, err, each.status)
			}
		})
	}
	t.Run("a dropped write", func(t *testing.T) {
		script := &scriptedTransport{replies: []func() (*http.Response, error){dropped, reply(200, "{}")}}
		transport := &retryTransport{next: script, baseDelay: time.Millisecond}
		if _, err := send(t, transport, "POST", "/graphql", `{"query":"mutation { x }"}`); err == nil || len(script.bodies) != 1 {
			t.Errorf("err = %v after %d attempts; a write must not be resent", err, len(script.bodies))
		}
	})
}

func TestExhaustedLimitHoldsRequestsUntilReset(t *testing.T) {
	reset := time.Now().Add(20 * time.Minute)
	limited := reply(200, `{"errors":[{"type":"RATE_LIMITED","message":"API rate limit exceeded"}]}`,
		"X-RateLimit-Resource", "graphql", "X-RateLimit-Remaining", "0", "X-RateLimit-Limit", "5000",
		"X-RateLimit-Reset", strconv.FormatInt(reset.Unix(), 10))
	script := &scriptedTransport{replies: []func() (*http.Response, error){limited}}
	transport := &retryTransport{next: script, baseDelay: time.Millisecond}

	response, err := send(t, transport, "POST", "/graphql", `{"query":"query { a }"}`)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := io.ReadAll(response.Body); !strings.Contains(string(body), "RATE_LIMITED") {
		t.Errorf("the caller should still see GitHub's error: %q", body)
	}
	health := transport.Health()
	if health.GraphQL.Remaining != 0 || health.GraphQL.Limit != 5000 || health.BlockedUntil.Before(reset.Add(-time.Second)) {
		t.Errorf("health = %+v", health)
	}

	_, err = send(t, transport, "GET", "/repos/o/r/pulls/1", "")
	var rateLimited RateLimitedError
	if !errors.As(err, &rateLimited) || len(script.bodies) != 1 {
		t.Errorf("held request: err = %v after %d sends", err, len(script.bodies))
	}
	if !strings.Contains(err.Error(), reset.Local().Format("15:04")) {
		t.Errorf("the error should say when the limit resets: %v", err)
	}
}

func TestLastRequestOfABudgetIsNotRefused(t *testing.T) {
	script := &scriptedTransport{replies: []func() (*http.Response, error){
		reply(200, `{"data":{}}`, "X-RateLimit-Resource", "graphql", "X-RateLimit-Remaining", "0", "X-RateLimit-Limit", "5000"),
	}}
	transport := &retryTransport{next: script, baseDelay: time.Millisecond}
	if _, err := send(t, transport, "POST", "/graphql", `{"query":"query { a }"}`); err != nil || len(script.bodies) != 1 {
		t.Errorf("err = %v after %d sends", err, len(script.bodies))
	}
	if !transport.Health().BlockedUntil.IsZero() {
		t.Error("a request that used the last point succeeded and should block nothing")
	}
}

func TestRESTBudgetIsRecorded(t *testing.T) {
	script := &scriptedTransport{replies: []func() (*http.Response, error){
		reply(200, "", "X-RateLimit-Resource", "core", "X-RateLimit-Remaining", "4000", "X-RateLimit-Limit", "5000"),
	}}
	transport := &retryTransport{next: script, baseDelay: time.Millisecond}
	send(t, transport, "GET", "/repos/o/r/pulls/1", "")
	if budget := transport.Health().REST; budget.Remaining != 4000 || budget.Limit != 5000 {
		t.Errorf("REST budget = %+v", budget)
	}
}

func TestRetryStopsWhenTheRequestIsCancelled(t *testing.T) {
	script := &scriptedTransport{replies: []func() (*http.Response, error){reply(503, "")}}
	transport := &retryTransport{next: script, baseDelay: 10 * time.Second}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	request, _ := http.NewRequestWithContext(cancelled, "GET", "https://api.github.com/x", nil)
	if _, err := transport.RoundTrip(request); err == nil {
		t.Error("a cancelled request should stop waiting")
	}
}
