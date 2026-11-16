package gh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RateLimit is the latest budget GitHub reported for one API resource.
type RateLimit struct {
	Remaining int
	Limit     int
	Reset     time.Time
}

// Health is what the client last learned about reaching GitHub.
type Health struct {
	GraphQL RateLimit
	REST    RateLimit
	// While set, requests fail at once instead of spending more of an exhausted limit.
	BlockedUntil time.Time
}

const (
	retryAttempts = 4
	// A longer wait is reported instead of blocking the caller on it.
	longestRetryWait = 30 * time.Second
)

// retryTransport retries what GitHub did not do (rate limits, overload, a
// dropped connection) with exponential backoff. A write is retried only when
// GitHub refused it outright, so it cannot land twice.
type retryTransport struct {
	next      http.RoundTripper
	baseDelay time.Duration

	mutex  sync.Mutex
	health Health
}

type RateLimitedError struct{ Until time.Time }

func (err RateLimitedError) Error() string {
	return fmt.Sprintf("GitHub rate limit exhausted until %s", err.Until.Local().Format("15:04"))
}

func (transport *retryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if until := transport.Health().BlockedUntil; time.Now().Before(until) {
		return nil, RateLimitedError{Until: until}
	}
	var body []byte
	if request.Body != nil {
		var err error
		if body, err = io.ReadAll(request.Body); err != nil {
			return nil, err
		}
		request.Body.Close()
	}
	var graphql struct{ Query string }
	isQuery := request.URL.Path == "/graphql" && json.Unmarshal(body, &graphql) == nil &&
		!strings.HasPrefix(strings.TrimSpace(graphql.Query), "mutation")
	idempotent := request.Method == http.MethodGet || isQuery

	for attempt := 1; ; attempt++ {
		attemptRequest := request.Clone(request.Context())
		attemptRequest.Body = io.NopCloser(bytes.NewReader(body))
		response, err := transport.next.RoundTrip(attemptRequest)
		wait := transport.baseDelay << (attempt - 1)
		retry := false
		switch {
		case err != nil:
			retry = idempotent
		case response.StatusCode == http.StatusTooManyRequests:
			retry = true
		case response.StatusCode >= 502 && response.StatusCode <= 504:
			retry = idempotent
		}
		if err == nil {
			limited, until := transport.observe(response)
			if limited {
				retry = true
				wait = max(wait, time.Until(until))
			}
			if seconds, parseErr := strconv.Atoi(response.Header.Get("Retry-After")); parseErr == nil {
				retry = true
				wait = max(wait, time.Duration(seconds)*time.Second)
			}
		}
		if !retry || attempt == retryAttempts || wait > longestRetryWait {
			if retry && wait > longestRetryWait {
				transport.mutex.Lock()
				transport.health.BlockedUntil = time.Now().Add(wait)
				transport.mutex.Unlock()
			}
			return response, err
		}
		if response != nil {
			io.Copy(io.Discard, response.Body)
			response.Body.Close()
		}
		select {
		case <-time.After(wait):
		case <-request.Context().Done():
			return nil, request.Context().Err()
		}
	}
}

// observe records the budget a response reports and whether it was refused
// for exceeding it. GraphQL reports that refusal with status 200, so its body is read.
func (transport *retryTransport) observe(response *http.Response) (limited bool, until time.Time) {
	header := response.Header
	remaining, err := strconv.Atoi(header.Get("X-RateLimit-Remaining"))
	if err != nil {
		return false, time.Time{}
	}
	limit, _ := strconv.Atoi(header.Get("X-RateLimit-Limit"))
	resetSeconds, _ := strconv.ParseInt(header.Get("X-RateLimit-Reset"), 10, 64)
	budget := RateLimit{Remaining: remaining, Limit: limit, Reset: time.Unix(resetSeconds, 0)}

	transport.mutex.Lock()
	if header.Get("X-RateLimit-Resource") == "graphql" {
		transport.health.GraphQL = budget
	} else {
		transport.health.REST = budget
	}
	transport.mutex.Unlock()

	if remaining > 0 {
		return false, time.Time{}
	}
	if response.StatusCode == http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		response.Body = io.NopCloser(bytes.NewReader(body))
		if !strings.Contains(string(body), `"RATE_LIMITED"`) {
			return false, time.Time{}
		}
	} else if response.StatusCode != http.StatusForbidden && response.StatusCode != http.StatusTooManyRequests {
		return false, time.Time{}
	}
	return true, budget.Reset
}

func (transport *retryTransport) Health() Health {
	transport.mutex.Lock()
	defer transport.mutex.Unlock()
	return transport.health
}
