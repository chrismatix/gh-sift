package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

// networkTopLine is the first screen line, where the status chip sits.
func networkTopLine(app *testApp) string {
	line, _, _ := strings.Cut(app.screen(), "\n")
	return line
}

func TestFailingRefreshBacksOffInsteadOfPollingEveryTick(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	if chip := app.statusChip(); chip != "" {
		t.Fatalf("a fresh PR needs no chip: %q", chip)
	}
	app.github.GraphQL("reviewThreads(first: 100)", ghtest.Reply(502, "{}"))
	data := app.data(app.pullRequest.key)
	data.detailAt = time.Now().Add(-time.Minute)

	app.send(tickMsg(time.Now()))
	failed := len(app.github.Find("reviewThreads(first: 100)"))
	if !strings.Contains(networkTopLine(app), "· retry in") {
		t.Fatalf("the chip should report the failure: %q", networkTopLine(app))
	}
	for range 5 {
		app.send(tickMsg(time.Now()))
	}
	if again := len(app.github.Find("reviewThreads(first: 100)")); again != failed {
		t.Errorf("ticks inside the backoff refetched: %d requests, then %d", failed, again)
	}

	// Once the backoff runs out, the next tick tries again, and success clears the chip.
	diffViewDefaultFixture().serve(app.github)
	app.network.retryAt = time.Now().Add(-time.Second)
	app.send(tickMsg(time.Now()))
	if app.network.failures != 0 || app.statusChip() != "" {
		t.Errorf("a successful refresh should clear the failure: %+v, chip %q", app.network, app.statusChip())
	}
}

func TestBackoffDoublesAndIsCapped(t *testing.T) {
	var state network
	var waits []time.Duration
	for range 10 {
		state.observe(errTest("boom"), gh.Health{})
		waits = append(waits, time.Until(state.retryAt).Round(time.Second))
	}
	if waits[0] != 5*time.Second || waits[1] != 10*time.Second || waits[2] != 20*time.Second || waits[9] != 5*time.Minute {
		t.Errorf("waits = %v", waits)
	}
	state.observe(nil, gh.Health{})
	if state.failures != 0 || !state.retryAt.IsZero() {
		t.Errorf("success should reset the backoff: %+v", state)
	}
}

func TestRateLimitHoldsPollingUntilReset(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	reset := time.Now().Add(30 * time.Minute)
	app.github.Headers = map[string]string{
		"X-RateLimit-Resource": "graphql", "X-RateLimit-Remaining": "0", "X-RateLimit-Limit": "5000",
		"X-RateLimit-Reset": strconv.FormatInt(reset.Unix(), 10),
	}
	app.github.GraphQL("reviewThreads(first: 100)", ghtest.Reply(200, `{"errors":[{"type":"RATE_LIMITED","message":"API rate limit exceeded"}]}`))
	app.press("R")

	resumes := "rate limited · resumes " + reset.Local().Format("15:04")
	if !strings.Contains(networkTopLine(app), resumes) {
		t.Fatalf("top line = %q, want %q", networkTopLine(app), resumes)
	}
	if app.network.retryAt.Before(reset.Add(-time.Second)) {
		t.Errorf("polling should wait for the reset, not %v", app.network.retryAt)
	}
	sent := len(app.github.Requests)
	app.press("R")
	if len(app.github.Requests) != sent {
		t.Error("a held request should not reach GitHub")
	}
}

func TestStaleAndLowBudgetChips(t *testing.T) {
	app := diffViewOpen(t, diffViewDefaultFixture())
	app.data(app.pullRequest.key).detailAt = time.Now().Add(-5 * time.Minute)
	if top := networkTopLine(app); !strings.Contains(top, "stale · synced 5m ago") {
		t.Errorf("top line = %q", top)
	}

	app.github.Headers = map[string]string{"X-RateLimit-Resource": "graphql", "X-RateLimit-Remaining": "312", "X-RateLimit-Limit": "5000", "X-RateLimit-Reset": "0"}
	app.press("R")
	if top := networkTopLine(app); !strings.Contains(top, "GraphQL 312/5000 left") {
		t.Errorf("top line = %q", top)
	}
}

func TestListRetriesAFailedSectionAfterTheBackoff(t *testing.T) {
	app := newTestApp(t, Options{})
	app.github.GraphQL("search(", ghtest.Reply(502, "{}"))
	app.press("r")
	if app.list.current().err == nil {
		t.Fatal("the section should have failed")
	}
	searches := len(app.github.Find("search("))
	app.send(tickMsg(time.Now()))
	if len(app.github.Find("search(")) != searches {
		t.Error("inside the backoff nothing should be retried")
	}
	app.github.GraphQL("search(", ghtest.Reply(200, map[string]any{"search": map[string]any{"nodes": []any{}, "issueCount": 0}}))
	app.network.retryAt = time.Now().Add(-time.Second)
	app.send(tickMsg(time.Now()))
	if app.list.current().err != nil || app.statusChip() != "" {
		t.Errorf("the failed section should have been retried: %v, chip %q", app.list.current().err, app.statusChip())
	}
}

type errTest string

func (err errTest) Error() string { return string(err) }
