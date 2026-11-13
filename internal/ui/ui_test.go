package ui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/chrismatix/gh-sift/internal/config"
	"github.com/chrismatix/gh-sift/internal/diff"
	"github.com/chrismatix/gh-sift/internal/gh"
)

func testStack() *gh.Stack {
	// Deliberately out of order: the API makes no promise about it.
	return &gh.Stack{Size: 3, BaseRefName: "main", Entries: []gh.StackEntry{
		{Position: 2, Number: 11, Title: "middle", State: "OPEN"},
		{Position: 3, Number: 12, Title: "top", State: "OPEN"},
		{Position: 1, Number: 10, Title: "bottom", State: "OPEN"},
	}}
}

func TestStackIsDrawnTopFirst(t *testing.T) {
	var numbers []int
	for _, entry := range topFirst(testStack()) {
		numbers = append(numbers, entry.Number)
	}
	if !reflect.DeepEqual(numbers, []int{12, 11, 10}) {
		t.Fatalf("order = %v, want top first", numbers)
	}

	lines := stackLines(NewTheme(true, nil), testStack(), 2, 80)
	plain := make([]string, len(lines))
	for index, line := range lines {
		plain[index] = ansi.Strip(line)
	}
	if !strings.Contains(plain[0], "#12 top") || !strings.Contains(plain[0], "▲ top") {
		t.Errorf("first line should be the top of the stack: %q", plain[0])
	}
	if !strings.HasPrefix(plain[1], "▸") || !strings.Contains(plain[1], "#11 middle") {
		t.Errorf("current PR should carry the marker: %q", plain[1])
	}
	if !strings.Contains(plain[2], "#10 bottom") || !strings.Contains(plain[2], "▼ merges first") {
		t.Errorf("last PR should be marked as merging first: %q", plain[2])
	}
	if !strings.Contains(plain[3], "main") {
		t.Errorf("the base branch should sit under the stack: %q", plain[3])
	}
}

func TestMergedBottomIsNotWhatMergesFirst(t *testing.T) {
	stack := testStack()
	stack.Entries[2].State = "MERGED"
	lines := stackLines(NewTheme(true, nil), stack, 3, 80)
	if !strings.Contains(ansi.Strip(lines[1]), "▼ merges first") {
		t.Errorf("the lowest open PR merges first: %q", ansi.Strip(lines[1]))
	}
}

func TestStackNeighbour(t *testing.T) {
	stack := testStack()
	if up := stack.Neighbour(2, 1); up == nil || up.Number != 12 {
		t.Errorf("up from the middle = %+v", up)
	}
	if down := stack.Neighbour(2, -1); down == nil || down.Number != 10 {
		t.Errorf("down from the middle = %+v", down)
	}
	if stack.Neighbour(3, 1) != nil || stack.Neighbour(1, -1) != nil {
		t.Error("the ends of the stack have no neighbour")
	}
}

func TestFileTreeJoinsSingleChildDirectories(t *testing.T) {
	files := []diff.File{
		{NewPath: "README.md"},
		{NewPath: "backend/app/src/main/a.go"},
		{NewPath: "backend/app/src/main/b.go"},
		{NewPath: "backend/app/src/test/c.go"},
	}
	var got []string
	for _, row := range fileTree(files) {
		got = append(got, strings.Repeat("  ", row.depth)+row.label)
	}
	want := []string{"README.md", "backend/app/src/", "  main/", "    a.go", "    b.go", "  test/", "    c.go"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tree =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestCustomKeybindingShadowsBuiltin(t *testing.T) {
	keys, err := newKeymap(config.Keybindings{Prs: []config.Keybinding{
		{Key: "W", Command: "echo agent", Name: "agent"},
		{Key: "M", Builtin: "merge"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if custom := matchCustom(keys.customPRs, "W"); custom == nil || custom.Name != "agent" {
		t.Errorf("W should run the custom command: %+v", custom)
	}
	if match(keys.prs, "M") != "merge" || match(keys.prs, "m") != "" {
		t.Error("a builtin rebind should replace the default key")
	}
	if _, err := newKeymap(config.Keybindings{Prs: []config.Keybinding{{Key: "Z", Builtin: "nope"}}}); err == nil {
		t.Error("an unknown builtin should be an error, as in gh-dash")
	}
}

func TestCleanBodyAndSeverity(t *testing.T) {
	body := "<!-- generated -->\n\n**<picture><img src=\"x.svg\" alt=\"\"></picture> Minor** · **Correctness**\n\n**Include a baseline.**\n\n<details>\n<summary>Prompt</summary>\nlong prompt\n</details>\n"
	if got := severity(body); got != "minor" {
		t.Errorf("severity = %q", got)
	}
	if got := summary(body); got != "Include a baseline." {
		t.Errorf("summary = %q", got)
	}
	if cleaned := cleanBody(body); strings.Contains(cleaned, "prompt") || strings.Contains(cleaned, "picture") {
		t.Errorf("cleanBody left hidden content: %q", cleaned)
	}
}

func testChecks() []gh.Check {
	job := func(name, workflow string, run int, conclusion string) gh.Check {
		check := gh.Check{Name: name, Workflow: workflow, RunID: run, JobID: run*10 + len(name), Status: "COMPLETED", Conclusion: conclusion, Event: "pull_request"}
		if conclusion == "" {
			check.Status = "IN_PROGRESS"
		}
		return check
	}
	return []gh.Check{
		job("lint", "CI", 1, "SUCCESS"),
		job("build", "Release", 2, ""),
		job("test", "CI", 1, "FAILURE"),
		{Name: "coderabbit", Status: "COMPLETED", Conclusion: "SUCCESS"},
		job("deploy", "Preview", 3, "SKIPPED"),
	}
}

func TestChecksGroupByRunWithFailuresFirst(t *testing.T) {
	var view checksView
	view.setChecks(testChecks())

	var names []string
	for _, group := range view.groups {
		names = append(names, group.name)
	}
	// Failing, then running, then the rest by name; a lone status is its own run.
	if want := []string{"CI", "Release", "coderabbit", "Preview"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("runs = %v, want %v", names, want)
	}
	if jobs := view.groups[0].jobs; len(jobs) != 2 || jobs[0].Name != "test" {
		t.Errorf("the failed job should lead its run: %+v", jobs)
	}
	if selected := view.selectedJob(); selected == nil || selected.Name != "test" {
		t.Errorf("the first thing selected should be what failed: %+v", selected)
	}
	if view.state() == nil {
		t.Error("an Actions job has steps and a log")
	}

	// A refresh that reorders the runs keeps the cursor on the same job.
	view.group, view.job = 1, 0
	refreshed := testChecks()
	refreshed[2].Conclusion = "SUCCESS"
	view.setChecks(refreshed)
	if selected := view.selectedJob(); selected == nil || selected.Name != "build" {
		t.Errorf("selection after refresh = %+v, want build", selected)
	}

	view.flat = true
	view.setChecks(testChecks())
	if len(view.groups) != 5 || view.groups[0].name != "test" || view.groups[0].detail != "CI" {
		t.Errorf("flat mode lists every check, failures first: %+v", view.groups)
	}
}

func TestStepIsFoundInLogByItsStartTime(t *testing.T) {
	at := func(second int) time.Time { return time.Date(2026, 11, 13, 12, 0, second, 0, time.UTC) }
	state := jobState{
		steps: []gh.JobStep{{Name: "Set up job", StartedAt: at(0)}, {Name: "Test", StartedAt: at(5)}, {Name: "Skipped"}},
		lines: []gh.LogLine{{Time: at(0)}, {Time: at(1)}, {Time: at(5).Add(300 * time.Millisecond)}, {Time: at(9)}},
		line:  3,
	}
	if got := state.lineForStep(0); got != 0 {
		t.Errorf("first step starts at line %d, want 0", got)
	}
	if got := state.lineForStep(1); got != 2 {
		t.Errorf("second step starts at line %d, want 2", got)
	}
	if got := state.lineForStep(2); got != 3 {
		t.Errorf("a step that never ran should leave the cursor alone, got %d", got)
	}
}
