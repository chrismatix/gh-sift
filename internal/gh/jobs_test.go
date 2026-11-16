package gh_test

import (
	"testing"
	"time"

	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

func TestJobSteps(t *testing.T) {
	server, client := ghtest.New(t)
	server.REST("GET", "repos/o/r/actions/jobs/900", ghtest.Reply(200, `{"id": 900, "steps": [
  {"name": "Set up job", "status": "completed", "conclusion": "success", "number": 1,
   "started_at": "2026-11-13T12:00:00Z", "completed_at": "2026-11-13T12:00:02Z"},
  {"name": "Run make test", "status": "in_progress", "conclusion": null, "number": 2,
   "started_at": "2026-11-13T12:00:02Z", "completed_at": null}
]}`))

	steps, err := client.JobSteps("o/r", 900)
	if err != nil {
		t.Fatal(err)
	}
	want := []gh.JobStep{
		{Name: "Set up job", Status: "COMPLETED", Conclusion: "SUCCESS",
			StartedAt: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC), CompletedAt: time.Date(2026, 11, 13, 12, 0, 2, 0, time.UTC)},
		{Name: "Run make test", Status: "IN_PROGRESS", StartedAt: time.Date(2026, 11, 13, 12, 0, 2, 0, time.UTC)},
	}
	if len(steps) != len(want) {
		t.Fatalf("steps = %+v", steps)
	}
	for index := range want {
		got := steps[index]
		if got.Name != want[index].Name || got.Status != want[index].Status || got.Conclusion != want[index].Conclusion ||
			!got.StartedAt.Equal(want[index].StartedAt) || !got.CompletedAt.Equal(want[index].CompletedAt) {
			t.Errorf("step %d = %+v, want %+v", index, got, want[index])
		}
	}

	if _, err := client.JobSteps("o/r", 901); err == nil {
		t.Error("an unknown job should be an error")
	}
}

func TestJobLog(t *testing.T) {
	server, client := ghtest.New(t)
	server.REST("GET", "repos/o/r/actions/jobs/900/logs", ghtest.Reply(200,
		"2026-11-13T12:00:05.0000000Z ##[group]Run make test\n2026-11-13T12:00:06.0000000Z ##[error]exit 1\n"))

	lines, err := client.JobLog("o/r", 900)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0].Kind != gh.LogStep || lines[1].Kind != gh.LogError || lines[1].Text != "exit 1" {
		t.Errorf("lines = %+v", lines)
	}

	// GitHub has no log for a running job yet.
	if _, err := client.JobLog("o/r", 901); err == nil {
		t.Error("a missing log should be an error")
	}
}
