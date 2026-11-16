package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

var prCheckTime = time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)

func prCheckRun(jobID, runID int, workflow, name, status, conclusion string) map[string]any {
	return map[string]any{
		"__typename": "CheckRun", "databaseId": jobID, "name": name, "status": status, "conclusion": conclusion,
		"startedAt": prCheckTime, "completedAt": prCheckTime.Add(90 * time.Second),
		"detailsUrl": fmt.Sprintf("https://github.com/o/r/actions/runs/%d/job/%d", runID, jobID),
		"checkSuite": map[string]any{"workflowRun": map[string]any{"databaseId": runID, "event": "pull_request", "workflow": map[string]any{"name": workflow}}},
	}
}

// prLog is a runner log whose last error is "second failure" on line 9.
var prLog = "2026-11-13T12:00:00.0000000Z ##[group]Run actions/checkout\n" +
	"2026-11-13T12:00:00.1000000Z [command]git fetch origin\n" +
	"2026-11-13T12:00:00.2000000Z ##[endgroup]\n" +
	"2026-11-13T12:00:05.0000000Z ##[group]Run go test ./...\n" +
	"2026-11-13T12:00:05.1000000Z ##[warning]a deprecated flag\n" +
	"2026-11-13T12:00:05.2000000Z ##[error]first failure\n" +
	"2026-11-13T12:00:05.3000000Z --- FAIL: TestSomething " + strings.Repeat("with a very long explanation ", 12) + "\n" +
	"2026-11-13T12:00:05.4000000Z ok  \tother/package\n" +
	"2026-11-13T12:00:05.5000000Z ##[error]second failure\n" +
	"2026-11-13T12:00:05.6000000Z ##[endgroup]\n" +
	"2026-11-13T12:00:06.0000000Z Cleaning up orphan processes\n"

// prChecksApp opens a PR whose checks are a failed CI run with two jobs, a
// running release job, a third-party check and a commit status.
func prChecksApp(t *testing.T) *testApp {
	t.Helper()
	detail := prDetail(7)
	thirdParty := prCheckRun(301, 0, "", "coderabbit", "COMPLETED", "SKIPPED")
	thirdParty["checkSuite"], thirdParty["title"], thirdParty["summary"] = map[string]any{}, "Review skipped", "Draft PRs are **not** reviewed."
	detail["commits"] = prNodes(map[string]any{"commit": map[string]any{"statusCheckRollup": map[string]any{"state": "FAILURE", "contexts": prNodes(
		prCheckRun(101, 1, "CI", "test", "COMPLETED", "FAILURE"),
		prCheckRun(102, 1, "CI", "lint", "COMPLETED", "SUCCESS"),
		prCheckRun(201, 2, "Release", "build", "IN_PROGRESS", ""),
		thirdParty,
		map[string]any{"__typename": "StatusContext", "context": "ci/legacy", "state": "SUCCESS", "createdAt": prCheckTime, "targetUrl": "https://ci.example.com"},
	)}}})
	app := prTestApp(t, detail)
	steps := map[string]any{"steps": []any{
		map[string]any{"name": "Set up job", "status": "completed", "conclusion": "success", "started_at": prCheckTime, "completed_at": prCheckTime.Add(time.Second)},
		map[string]any{"name": "Run go test", "status": "completed", "conclusion": "failure", "started_at": prCheckTime.Add(5 * time.Second), "completed_at": prCheckTime.Add(6 * time.Second)},
		map[string]any{"name": "Post checkout", "status": "completed", "conclusion": "skipped"},
	}}
	app.github.REST("GET", "repos/o/r/actions/jobs/", ghtest.Reply(200, steps))
	app.github.REST("GET", "repos/o/r/actions/jobs/201", ghtest.Reply(200, map[string]any{"steps": []any{
		map[string]any{"name": "Set up job", "status": "completed", "conclusion": "success", "started_at": prCheckTime, "completed_at": prCheckTime.Add(time.Second)},
		map[string]any{"name": "Build", "status": "in_progress", "started_at": prCheckTime.Add(time.Second)},
		map[string]any{"name": "Upload", "status": "queued"},
	}}))
	app.github.REST("GET", "repos/o/r/actions/jobs/101/logs", ghtest.Reply(200, prLog))
	app.github.REST("GET", "repos/o/r/actions/jobs/102/logs", ghtest.Reply(200, ""))
	app.press("2")
	return app
}

// prSettle delivers the timer the checks tab sets when the cursor stops,
// which the harness drops for being slow.
func prSettle(app *testApp) {
	app.send(checksSettledMsg{settle: app.pullRequest.checks.settle})
}

func TestChecksTabOpensOnTheLastErrorOfTheFailedJob(t *testing.T) {
	app := prChecksApp(t)
	app.expectScreen(
		"Checks ✓ 3  ✗ 1  ● 1",
		"Runs", "4 runs", "CI", "on pull_request",
		"Jobs", "2 jobs", "test", "1m 30s",
		"Steps", "3 steps", "Set up job", "Run go test", "Post checko",
		"Job Logs", "/ search · e/E error · ] [ group",
		"▶ Run actions/checkout", "git fetch origin", "Warning: a deprecated flag", "Error: second failure",
		"81% (9/11)",
		"5 checks: 1 failing, 1 in progress, 2 successful, 1 skipped",
	)
	state := app.pullRequest.checks.state()
	if state.line != 8 || state.step != 1 {
		t.Errorf("cursor at line %d step %d, want the last error (8) and the failed step (1)", state.line, state.step)
	}
	if len(app.github.Find("actions/jobs/101/logs")) != 1 {
		t.Error("the failed job's log should be fetched once")
	}
}

func TestChecksLogNavigationAndSearch(t *testing.T) {
	app := prChecksApp(t)
	checks := &app.pullRequest.checks
	line := func() int { return checks.state().line }
	app.press("lll")
	if checks.focus != paneLogs {
		t.Fatalf("lll should focus the log, focus = %d", checks.focus)
	}
	app.expectScreen("j/k line · / search")

	steps := []struct {
		keys string
		line int
	}{
		{"E", 5}, {"E", 5}, {"e", 8}, {"[", 3}, {"[", 0}, {"]", 3}, {"k", 2}, {"G", 10}, {"g", 0}, {"j", 1},
	}
	for _, step := range steps {
		app.press(step.keys)
		if line() != step.line {
			t.Fatalf("after %s the cursor is on line %d, want %d", step.keys, line(), step.line)
		}
	}

	app.press("/")
	app.expectScreen("type to search the log · enter find · esc cancel")
	app.press("FAILX", "backspace", "space", "backspace", "enter")
	if checks.query != "FAIL" || line() != 5 {
		t.Fatalf("searching FAIL: query %q, line %d, want the first match on line 5", checks.query, line())
	}
	app.expectScreen("/FAIL  n/N next · esc clear")
	app.press("n")
	if line() != 6 {
		t.Errorf("n should find the next match, line %d", line())
	}
	app.press("n", "n")
	prExpectToast(t, app, "no further match")
	app.press("N")
	if line() != 6 {
		t.Errorf("N should go back a match, line %d", line())
	}
	app.press("esc")
	if checks.query != "" {
		t.Error("esc should clear the search")
	}
	app.press("/", "x", "esc")
	if checks.typing || checks.query != "" {
		t.Error("esc while typing should cancel the search")
	}
	// Esc with nothing to undo is the PR view's: opened directly, it stays.
	app.press("esc")
	if app.pullRequest.tab != tabChecks {
		t.Error("esc should leave the checks tab alone")
	}
}

func TestChecksStepsMoveTheLog(t *testing.T) {
	app := prChecksApp(t)
	app.press("ll")
	state := app.pullRequest.checks.state()
	app.press("k")
	if state.step != 0 || state.line != 0 {
		t.Errorf("the first step starts the log: step %d line %d", state.step, state.line)
	}
	app.press("j")
	if state.step != 1 || state.line != 3 {
		t.Errorf("the second step starts at its group: step %d line %d", state.step, state.line)
	}
}

func TestChecksMovingBetweenJobsAndRuns(t *testing.T) {
	app := prChecksApp(t)
	checks := &app.pullRequest.checks

	// The cursor rests on the passing lint job, whose log is empty.
	app.press("l", "j")
	prSettle(app)
	app.expectScreen("The log is empty.", "3 steps")
	app.press("/")
	prExpectToast(t, app, "this check has no log to search")
	app.press("e")
	if checks.focus != paneJobs {
		t.Error("e without a log should do nothing")
	}

	// A stale timer is ignored.
	stale := checks.settle
	app.press("k")
	app.send(checksSettledMsg{settle: stale})
	if job := checks.selectedJob(); job.Name != "test" {
		t.Fatalf("selected %s, want test", job.Name)
	}

	// The running release job has no log yet, and its step follows the job.
	app.press("h", "j")
	prSettle(app)
	app.expectScreen("Release", "running", "GitHub publishes the log when the job ends", "● Build")
	if state := checks.state(); state.step != 1 {
		t.Errorf("the running step should be selected, step %d", state.step)
	}
	if len(app.github.Find("actions/jobs/201/logs")) != 0 {
		t.Error("a running job's log does not exist yet")
	}

	// Its steps are polled from the periodic tick.
	checks.state().stepsAt = time.Now().Add(-time.Minute)
	requests := len(app.github.Find("actions/jobs/201"))
	app.send(tickMsg(time.Now()))
	if len(app.github.Find("actions/jobs/201")) != requests+1 {
		t.Error("a running job's steps should be polled")
	}

	// A commit status has no steps or log, only a link.
	app.press("j")
	prSettle(app)
	app.expectScreen("ci/legacy", "no steps", "Not an Actions job, so no log.")
	app.press("o")
	if app.toast.failure {
		t.Errorf("opening the check failed: %s", app.toast.text)
	}

	// A third-party check shows its own summary instead.
	app.press("G")
	prSettle(app)
	app.expectScreen("coderabbit", "skipped", "Review skipped", "Draft PRs are not reviewed.")
}

func TestChecksLogFailsToLoad(t *testing.T) {
	app := prChecksApp(t)
	app.github.REST("GET", "repos/o/r/actions/jobs/101/logs", ghtest.Reply(500, map[string]any{"message": "boom"}))
	app.pullRequest.checks.jobs = map[int]*jobState{}
	app.send(tickMsg(time.Now()))
	app.expectScreen("Could not fetch the log")
}

func TestChecksZoomFlatAndOpen(t *testing.T) {
	app := prChecksApp(t)
	checks := &app.pullRequest.checks

	app.press("z")
	app.expectScreen("Runs", "4 runs")
	if strings.Contains(app.screen(), "Job Logs") {
		t.Error("zoom shows only the focused pane")
	}
	app.press("l", "l")
	if checks.focus != paneSteps {
		t.Errorf("zoomed, l still moves through the panes, focus %d", checks.focus)
	}
	app.press("esc")
	if checks.zoom {
		t.Error("esc should leave the zoom")
	}

	app.press("h", "f")
	if !checks.flat || checks.focus != paneRuns {
		t.Errorf("f should flatten and move off the jobs pane, flat %v focus %d", checks.flat, checks.focus)
	}
	app.expectScreen("Checks", "5 checks", "test", "CI")
	if strings.Contains(app.screen(), "Jobs") {
		t.Error("the flat view has no jobs pane")
	}
	app.press("f")
	app.expectScreen("Jobs")

	app.press("o", "O")
	if app.toast.failure {
		t.Errorf("opening failed: %s", app.toast.text)
	}
}

func TestChecksRerunFailed(t *testing.T) {
	app := prChecksApp(t)
	app.press("ctrl+r")
	if !app.toast.failure {
		t.Errorf("an unanswered rerun should fail, toast %q", app.toast.text)
	}

	app.github.REST("POST", "repos/o/r/actions/runs/1/rerun-failed-jobs", ghtest.Reply(201, map[string]any{}))
	app.press("u")
	prExpectToast(t, app, "rerunning 1 failed run")
	if found := app.github.Find("actions/runs/1/rerun-failed-jobs"); len(found) != 2 {
		t.Errorf("rerun requests = %d, want 2", len(found))
	}

	app.pullRequest.checks.setChecks(nil)
	app.press("ctrl+r")
	prExpectToast(t, app, "no failed workflow runs to rerun")
	app.expectScreen("No checks reported.")
}

func TestChecksWithoutJobsShowLoadingStates(t *testing.T) {
	app := prChecksApp(t)
	checks := &app.pullRequest.checks
	state := checks.state()
	state.steps, state.loaded = nil, false
	app.expectScreen("loading", "loading the log…")
}
