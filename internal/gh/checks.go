package gh

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type JobStep struct {
	Name        string
	Status      string
	Conclusion  string
	StartedAt   time.Time
	CompletedAt time.Time
}

func (client *Client) JobSteps(repository string, jobID int) ([]JobStep, error) {
	var response struct {
		Steps []struct {
			Name        string
			Status      string
			Conclusion  string
			StartedAt   time.Time `json:"started_at"`
			CompletedAt time.Time `json:"completed_at"`
		}
	}
	if err := client.rest.Get(fmt.Sprintf("repos/%s/actions/jobs/%d", repository, jobID), &response); err != nil {
		return nil, err
	}
	steps := make([]JobStep, 0, len(response.Steps))
	for _, step := range response.Steps {
		steps = append(steps, JobStep{
			Name: step.Name, Status: strings.ToUpper(step.Status), Conclusion: strings.ToUpper(step.Conclusion),
			StartedAt: step.StartedAt, CompletedAt: step.CompletedAt,
		})
	}
	return steps, nil
}

type LogKind int

const (
	LogText LogKind = iota
	// LogStep opens a step's own group ("##[group]Run ..."), LogGroup any other.
	LogStep
	LogGroup
	LogCommand
	LogWarning
	LogError
)

type LogLine struct {
	Time time.Time
	// Text may carry SGR colour escapes from the job's own output.
	Text string
	Kind LogKind
	// How many groups the line sits inside.
	Depth int
}

// JobLog fetches a finished job's log. GitHub has no log for a job that is
// still running: the endpoint answers 404 until it completes.
func (client *Client) JobLog(repository string, jobID int) ([]LogLine, error) {
	raw, err := client.getText(fmt.Sprintf("repos/%s/actions/jobs/%d/logs", repository, jobID), "")
	if err != nil {
		return nil, err
	}
	return ParseLog(raw), nil
}

var (
	logTimestamp = regexp.MustCompile(`^(\d{4}-\d\d-\d\dT[\d:.]+Z) ?`)
	// Everything except colour: cursor movement would corrupt the screen.
	nonColourEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-ln-z]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)|\r`)
)

// ParseLog reads the runner's log format: a timestamp per line and
// "##[...]" markers for groups, commands, warnings and errors.
func ParseLog(raw string) []LogLine {
	var lines []LogLine
	var last time.Time
	depth := 0
	for _, text := range strings.Split(strings.TrimPrefix(raw, "\uFEFF"), "\n") {
		if match := logTimestamp.FindStringSubmatch(text); match != nil {
			if parsed, err := time.Parse(time.RFC3339Nano, match[1]); err == nil {
				last = parsed
			}
			text = text[len(match[0]):]
		}
		text = nonColourEscape.ReplaceAllString(text, "")
		text = strings.ReplaceAll(text, "\t", "    ")
		line := LogLine{Time: last, Text: text, Depth: depth}
		switch {
		case strings.HasPrefix(text, "##[group]"):
			line.Text, line.Kind = strings.TrimPrefix(text, "##[group]"), LogGroup
			if strings.HasPrefix(line.Text, "Run ") {
				line.Kind = LogStep
			}
			depth++
		case strings.HasPrefix(text, "##[endgroup]"):
			// The closing marker becomes the blank line that sets groups apart.
			depth = max(depth-1, 0)
			line.Text, line.Depth = "", depth
		case strings.HasPrefix(text, "[command]"):
			line.Text, line.Kind = strings.TrimPrefix(text, "[command]"), LogCommand
		case strings.HasPrefix(text, "##[warning]"):
			line.Text, line.Kind = strings.TrimPrefix(text, "##[warning]"), LogWarning
		case strings.HasPrefix(text, "##[error]"):
			line.Text, line.Kind = strings.TrimPrefix(text, "##[error]"), LogError
		}
		lines = append(lines, line)
	}
	for len(lines) > 0 && lines[len(lines)-1].Text == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func (client *Client) RerunFailedJobs(repository string, runID int) error {
	return client.post(fmt.Sprintf("repos/%s/actions/runs/%d/rerun-failed-jobs", repository, runID), struct{}{}, nil)
}
