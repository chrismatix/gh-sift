package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chrismatix/gh-sift/internal/gh"
)

// The Checks tab is four panes side by side, after dlvhdr/gh-enhance:
// workflow runs, the selected run's jobs, the job's steps, and its log.
type checksPane int

const (
	paneRuns checksPane = iota
	paneJobs
	paneSteps
	paneLogs
)

// checkGroup is one row of the Runs pane: a workflow run with its jobs, or
// a check that stands alone (a commit status, a third-party app).
type checkGroup struct {
	name   string
	detail string
	jobs   []gh.Check
}

// jobState is what has been fetched for one job and where the reader left
// it, so coming back to a job restores its step and log position.
type jobState struct {
	steps   []gh.JobStep
	stepsAt time.Time
	lines   []gh.LogLine
	loading bool
	loaded  bool
	err     error
	step    int
	line    int
	top     int
}

// logRow is one screen row of the log; a long line wraps into several.
type logRow struct {
	line int
	text string
}

type checksView struct {
	focus  checksPane
	zoom   bool
	flat   bool
	groups []checkGroup
	group  int
	job    int
	jobs   map[int]*jobState

	rows      []logRow
	rowStart  []int
	rowsJob   int
	rowsWidth int
	rowsCount int

	query  string
	typing bool
	// Bumped on every cursor move, so only the job the cursor rests on is fetched.
	settle int
}

type (
	jobDetailMsg struct {
		jobID int
		steps []gh.JobStep
		err   error
	}
	jobLogMsg struct {
		jobID int
		lines []gh.LogLine
		err   error
	}
	checksSettledMsg struct{ settle int }
)

// checkRank orders checks by how much they want attention.
func checkRank(check gh.Check) int {
	switch {
	case check.Failed():
		return 0
	case check.Running():
		return 1
	}
	return 2
}

func (group checkGroup) rank() int {
	rank := 2
	for _, job := range group.jobs {
		rank = min(rank, checkRank(job))
	}
	return rank
}

// setChecks rebuilds the panes from fresh data, keeping the selection on
// the same job. The first time, it selects what failed.
func (view *checksView) setChecks(checks []gh.Check) {
	if view.jobs == nil {
		view.jobs = map[int]*jobState{}
	}
	var selectedName string
	selectedID, hadSelection := 0, false
	if job := view.selectedJob(); job != nil {
		selectedName, selectedID, hadSelection = job.Name, job.JobID, true
	}

	view.groups = nil
	byRun := map[int]int{}
	for _, check := range checks {
		index, known := byRun[check.RunID]
		if view.flat || !check.IsJob() || !known {
			group := checkGroup{name: check.Workflow, detail: "status check"}
			switch {
			case view.flat && check.IsJob():
				group.name, group.detail = check.Name, check.Workflow
			case check.IsJob():
				group.detail = "on " + check.Event + " · " + ago(check.StartedAt)
				byRun[check.RunID] = len(view.groups)
			default:
				group.name = check.Name
			}
			view.groups = append(view.groups, group)
			index = len(view.groups) - 1
		}
		view.groups[index].jobs = append(view.groups[index].jobs, check)
	}
	slices.SortStableFunc(view.groups, func(first, second checkGroup) int {
		return cmp.Or(first.rank()-second.rank(), strings.Compare(strings.ToLower(first.name), strings.ToLower(second.name)))
	})
	for _, group := range view.groups {
		slices.SortStableFunc(group.jobs, func(first, second gh.Check) int { return checkRank(first) - checkRank(second) })
	}

	view.group, view.job = 0, 0
	for groupIndex, group := range view.groups {
		for jobIndex, job := range group.jobs {
			if hadSelection && job.Name == selectedName && job.JobID == selectedID {
				view.group, view.job = groupIndex, jobIndex
			}
		}
	}
}

func (view *checksView) selectedJob() *gh.Check {
	if view.group < len(view.groups) && view.job < len(view.groups[view.group].jobs) {
		return &view.groups[view.group].jobs[view.job]
	}
	return nil
}

// state returns the selected job's fetched data, or nil for a check that is
// not an Actions job and so has neither steps nor a log.
func (view *checksView) state() *jobState {
	job := view.selectedJob()
	if job == nil || !job.IsJob() {
		return nil
	}
	if view.jobs[job.JobID] == nil {
		view.jobs[job.JobID] = &jobState{}
	}
	return view.jobs[job.JobID]
}

// load fetches what the selected job is missing. A running job's steps are
// polled; its log does not exist on GitHub until the job ends.
func (view *checksView) load(app *App) tea.Cmd {
	job, state := view.selectedJob(), view.state()
	if state == nil {
		return nil
	}
	repository, jobID, client := app.pullRequest.key.repository, job.JobID, app.client
	var commands []tea.Cmd
	if state.stepsAt.IsZero() || (job.Running() && time.Since(state.stepsAt) > 5*time.Second) {
		state.stepsAt = time.Now()
		commands = append(commands, func() tea.Msg {
			steps, err := client.JobSteps(repository, jobID)
			return jobDetailMsg{jobID: jobID, steps: steps, err: err}
		})
	}
	if !job.Running() && !state.loaded && !state.loading {
		state.loading = true
		commands = append(commands, func() tea.Msg {
			lines, err := client.JobLog(repository, jobID)
			return jobLogMsg{jobID: jobID, lines: lines, err: err}
		})
	}
	return tea.Batch(commands...)
}

// settled schedules a load for shortly after the cursor stops moving.
func (view *checksView) settled() tea.Cmd {
	view.settle++
	message := checksSettledMsg{settle: view.settle}
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return message })
}

func (view *checksView) stepsLoaded(message jobDetailMsg) {
	state := view.jobs[message.jobID]
	if state == nil || message.err != nil {
		return
	}
	// A cursor on the running step follows the job as it moves on.
	following := state.steps == nil || (state.step < len(state.steps) && state.steps[state.step].Status == "IN_PROGRESS")
	state.steps = message.steps
	if following {
		// Open on the step that failed, like the log opens on its error.
		interesting := slices.IndexFunc(state.steps, func(step gh.JobStep) bool {
			return step.Conclusion == "FAILURE" || step.Status == "IN_PROGRESS"
		})
		if interesting >= 0 {
			state.step = interesting
		}
	}
	state.step = clamp(state.step, 0, max(len(state.steps)-1, 0))
}

func (view *checksView) logLoaded(app *App, message jobLogMsg) {
	state := view.jobs[message.jobID]
	if state == nil {
		return
	}
	state.loading, state.loaded, state.lines, state.err = false, true, message.lines, message.err
	state.line = 0
	for index := len(state.lines) - 1; index >= 0; index-- {
		if state.lines[index].Kind == gh.LogError {
			state.line = index
			break
		}
	}
	view.rowsJob = 0
	// Show what led up to the error: put it near the bottom of the pane.
	view.place(app, state, 0.8)
}

// panes lists the visible panes left to right with their widths. The
// focused list pane is wider; the log takes what is left.
func (view *checksView) panes(app *App) ([]checksPane, []int) {
	order := []checksPane{paneRuns, paneJobs, paneSteps, paneLogs}
	if view.flat {
		order = []checksPane{paneRuns, paneSteps, paneLogs}
	}
	if view.zoom {
		return []checksPane{view.focus}, []int{app.width}
	}
	wide, narrow := 38, 22
	if app.width < 150 {
		wide, narrow = 30, 17
	}
	widths := make([]int, len(order))
	remaining := app.width
	for index, pane := range order[:len(order)-1] {
		widths[index] = narrow
		if pane == view.focus {
			widths[index] = wide
		}
		remaining -= widths[index] + 1
	}
	widths[len(order)-1] = remaining
	return order, widths
}

func (view *checksView) logWidth(app *App) int {
	order, widths := view.panes(app)
	if order[len(order)-1] != paneLogs {
		return 0
	}
	return widths[len(widths)-1]
}

// Screen rows the panes get: everything under the PR header and tab row.
func (view *checksView) height(app *App) int { return max(app.height-8, 6) }

// Rows of log text: the pane minus its title, search line and position line.
func (view *checksView) logHeight(app *App) int { return max(view.height(app)-3, 1) }

// ensureRows wraps the selected job's log to the pane, once per width.
func (view *checksView) ensureRows(app *App, state *jobState, jobID int) {
	limit := view.logWidth(app)
	if view.rowsJob == jobID && view.rowsWidth == limit && view.rowsCount == len(state.lines) {
		return
	}
	theme := app.theme
	view.rowsJob, view.rowsWidth, view.rowsCount = jobID, limit, len(state.lines)
	view.rows, view.rowStart = view.rows[:0], make([]int, len(state.lines))
	digits := len(strconv.Itoa(len(state.lines)))
	textWidth := max(limit-digits-5, 10)
	for index, line := range state.lines {
		guide := strings.Repeat("│  ", line.Depth)
		body := line.Text
		switch line.Kind {
		case gh.LogStep:
			body = theme.text(theme.Focus).Bold(true).Render("▶ " + ansi.Strip(body))
		case gh.LogGroup:
			body = theme.text(theme.Focus).Render("▶ " + ansi.Strip(body))
		case gh.LogCommand:
			body = theme.text(theme.Focus).Render(ansi.Strip(body))
		case gh.LogWarning:
			body = theme.warning().Bold(true).Render("Warning: ") + theme.warning().Render(ansi.Strip(body))
		case gh.LogError:
			body = theme.failure().Bold(true).Render("Error: ") + theme.failure().Render(ansi.Strip(body))
		}
		view.rowStart[index] = len(view.rows)
		room := max(textWidth-width(guide), 10)
		// Most lines fit; wrapping is the slow path and logs run to tens of thousands of lines.
		pieces := []string{body}
		if width(body) > room {
			pieces = strings.Split(lipgloss.Wrap(body, room, ""), "\n")
		}
		for _, piece := range pieces {
			view.rows = append(view.rows, logRow{line: index, text: theme.text(theme.FaintBorder).Render(guide) + piece + ansi.ResetStyle})
		}
	}
}

// place scrolls so the cursor line sits a given fraction down the pane.
func (view *checksView) place(app *App, state *jobState, fraction float64) {
	job := view.selectedJob()
	if job == nil || len(state.lines) == 0 {
		return
	}
	view.ensureRows(app, state, job.JobID)
	state.top = view.rowStart[state.line] - int(float64(view.logHeight(app))*fraction)
}

// lineForStep finds where a step begins in the log. The log does not name
// its steps, so the step's start time is matched against line timestamps.
func (state *jobState) lineForStep(index int) int {
	started := state.steps[index].StartedAt
	if started.IsZero() || len(state.lines) == 0 {
		return state.line
	}
	for line := range state.lines {
		if !state.lines[line].Time.Before(started) {
			return line
		}
	}
	return len(state.lines) - 1
}

func (view *checksView) matches(line gh.LogLine) bool {
	return view.query != "" && strings.Contains(strings.ToLower(ansi.Strip(line.Text)), strings.ToLower(view.query))
}

// seek moves the log cursor to the next line in a direction that passes the test.
func (view *checksView) seek(app *App, state *jobState, direction int, what string, test func(gh.LogLine) bool) {
	for index := state.line + direction; index >= 0 && index < len(state.lines); index += direction {
		if test(state.lines[index]) {
			state.line = index
			return
		}
	}
	app.notify(map[int]string{1: "no further ", -1: "no earlier "}[direction]+what, false)
}

// handleKey reports whether the key was the Checks tab's to handle.
func (view *checksView) handleKey(app *App, pressed string) (bool, tea.Cmd) {
	state := view.state()
	if view.typing {
		switch {
		case pressed == "esc":
			view.typing, view.query = false, ""
		case pressed == "enter":
			view.typing = false
			if state != nil && len(state.lines) > 0 && !view.matches(state.lines[state.line]) {
				view.seek(app, state, 1, "match", view.matches)
			}
		case pressed == "backspace":
			if view.query != "" {
				_, size := utf8.DecodeLastRuneInString(view.query)
				view.query = view.query[:len(view.query)-size]
			}
		case pressed == "space":
			view.query += " "
		case utf8.RuneCountInString(pressed) == 1:
			view.query += pressed
		}
		return true, nil
	}

	order, _ := view.panes(app)
	if view.zoom {
		order = []checksPane{paneRuns, paneJobs, paneSteps, paneLogs}
		if view.flat {
			order = []checksPane{paneRuns, paneSteps, paneLogs}
		}
	}
	position := slices.Index(order, view.focus)
	half := max(view.logHeight(app)/2, 1)
	move := map[string]int{"j": 1, "down": 1, "k": -1, "up": -1, "ctrl+d": half, "ctrl+u": -half, "g": -1 << 30, "home": -1 << 30, "G": 1 << 30, "end": 1 << 30}[pressed]

	switch pressed {
	case "h", "left":
		view.focus = order[max(position-1, 0)]
	case "l", "right", "enter":
		view.focus = order[min(position+1, len(order)-1)]
	case "z":
		view.zoom = !view.zoom
	case "f":
		view.flat = !view.flat
		if view.flat && view.focus == paneJobs {
			view.focus = paneRuns
		}
		if detail := app.pullRequest.detail(app); detail != nil {
			view.setChecks(detail.Checks)
		}
		return true, view.load(app)
	case "/":
		if state == nil || len(state.lines) == 0 {
			app.notify("this check has no log to search", false)
			return true, nil
		}
		view.focus, view.typing, view.query = paneLogs, true, ""
	case "esc":
		switch {
		case view.zoom:
			view.zoom = false
		case view.query != "":
			view.query = ""
		default:
			return false, nil
		}
	case "ctrl+r", "u":
		return true, view.rerunFailed(app)
	case "o":
		if job := view.selectedJob(); job != nil && job.URL != "" {
			return true, app.openInBrowser(job.URL)
		}
	case "O":
		if row := app.pullRequest.row(app); row != nil {
			return true, app.openInBrowser(row.URL)
		}
	case "n", "N", "e", "E", "]", "[":
		if state == nil || len(state.lines) == 0 {
			return true, nil
		}
		direction := 1
		if pressed == "N" || pressed == "E" || pressed == "[" {
			direction = -1
		}
		switch pressed {
		case "n", "N":
			view.seek(app, state, direction, "match", view.matches)
		case "e", "E":
			view.seek(app, state, direction, "error", func(line gh.LogLine) bool { return line.Kind == gh.LogError })
		default:
			view.seek(app, state, direction, "group", func(line gh.LogLine) bool { return line.Kind == gh.LogStep || line.Kind == gh.LogGroup })
		}
	default:
		if move == 0 {
			return false, nil
		}
		switch view.focus {
		case paneRuns:
			view.group = clamp(view.group+move, 0, max(len(view.groups)-1, 0))
			view.job = 0
			return true, view.settled()
		case paneJobs:
			if view.group < len(view.groups) {
				view.job = clamp(view.job+move, 0, max(len(view.groups[view.group].jobs)-1, 0))
			}
			return true, view.settled()
		case paneSteps:
			if state != nil && len(state.steps) > 0 {
				state.step = clamp(state.step+move, 0, len(state.steps)-1)
				state.line = state.lineForStep(state.step)
				// A step's start reads best from the top of the pane.
				view.place(app, state, 0.1)
			}
		case paneLogs:
			if state != nil {
				state.line = clamp(state.line+move, 0, max(len(state.lines)-1, 0))
			}
		}
	}
	return true, nil
}

func (view *checksView) rerunFailed(app *App) tea.Cmd {
	var runs []int
	for _, group := range view.groups {
		for _, check := range group.jobs {
			if check.Failed() && check.IsJob() && !slices.Contains(runs, check.RunID) {
				runs = append(runs, check.RunID)
			}
		}
	}
	if len(runs) == 0 {
		app.notify("no failed workflow runs to rerun", false)
		return nil
	}
	key := app.pullRequest.key
	return app.run("rerunning", func() error {
		for _, run := range runs {
			if err := app.client.RerunFailedJobs(key.repository, run); err != nil {
				return err
			}
		}
		return nil
	}, "rerunning "+plural(len(runs), "failed run"), func(app *App) tea.Cmd { return app.loadPullRequest(key, true) })
}

// summary is the one-line tally under the panes, as gh-enhance words it.
func (view *checksView) summary(theme Theme) string {
	failing, running, successful, skipped := 0, 0, 0, 0
	for _, group := range view.groups {
		for _, check := range group.jobs {
			switch {
			case check.Failed():
				failing++
			case check.Running():
				running++
			case check.Skipped():
				skipped++
			default:
				successful++
			}
		}
	}
	total := failing + running + successful + skipped
	parts := []string{}
	if failing > 0 {
		parts = append(parts, theme.failure().Render(fmt.Sprintf("%d failing", failing)))
	}
	if running > 0 {
		parts = append(parts, theme.warning().Render(fmt.Sprintf("%d in progress", running)))
	}
	if successful > 0 {
		parts = append(parts, theme.success().Render(fmt.Sprintf("%d successful", successful)))
	}
	if skipped > 0 {
		parts = append(parts, theme.faint().Render(fmt.Sprintf("%d skipped", skipped)))
	}
	return theme.secondary().Render(plural(total, "check")+": ") + strings.Join(parts, theme.faint().Render(", "))
}

func checkIcon(theme Theme, check gh.Check) string {
	switch {
	case check.Running():
		return theme.warning().Render("●")
	case check.Failed():
		return theme.failure().Render("✗")
	case check.Conclusion == "SUCCESS":
		return theme.success().Render("✓")
	}
	return theme.faint().Render("⊘")
}

func stepIcon(theme Theme, step gh.JobStep) string {
	switch {
	case step.Status == "IN_PROGRESS":
		return theme.warning().Render("●")
	case step.Status != "COMPLETED":
		return theme.faint().Render("○")
	case step.Conclusion == "FAILURE":
		return theme.failure().Render("✗")
	case step.Conclusion == "SUCCESS":
		return theme.success().Render("✓")
	}
	return theme.faint().Render("⊘")
}

type paneItem struct {
	icon   string
	name   string
	detail string
}

func (view *checksView) view(app *App, height int) []string {
	theme := app.theme
	if len(view.groups) == 0 {
		return []string{"", "  " + theme.faint().Render("No checks reported.")}
	}
	order, widths := view.panes(app)
	job, state := view.selectedJob(), view.state()

	var columns [][]string
	for index, pane := range order {
		focused := pane == view.focus
		var lines []string
		switch pane {
		case paneRuns:
			title, unit := "Runs", "run"
			if view.flat {
				title, unit = "Checks", "check"
			}
			items := make([]paneItem, len(view.groups))
			for groupIndex, group := range view.groups {
				// A run shows its worst job's state.
				worst := group.jobs[0]
				for _, each := range group.jobs {
					if checkRank(each) < checkRank(worst) {
						worst = each
					}
				}
				items[groupIndex] = paneItem{checkIcon(theme, worst), group.name, group.detail}
			}
			lines = listPane(theme, title, plural(len(items), unit), items, view.group, focused, widths[index], height)
		case paneJobs:
			var items []paneItem
			for _, each := range view.groups[view.group].jobs {
				detail := duration(each.StartedAt, each.CompletedAt)
				switch {
				case each.Skipped():
					detail = strings.ToLower(each.Conclusion)
				case each.Running():
					detail = "running " + detail
				}
				items = append(items, paneItem{checkIcon(theme, each), each.Name, detail})
			}
			lines = listPane(theme, "Jobs", plural(len(items), "job"), items, view.job, focused, widths[index], height)
		case paneSteps:
			var items []paneItem
			cursor, count := 0, "no steps"
			if state != nil {
				cursor = state.step
				for _, step := range state.steps {
					detail := duration(step.StartedAt, step.CompletedAt)
					if step.Status == "IN_PROGRESS" {
						detail = "running " + detail
					}
					items = append(items, paneItem{stepIcon(theme, step), step.Name, detail})
				}
				count = plural(len(items), "step")
				if state.steps == nil {
					count = app.spinner() + " loading"
				}
			}
			lines = listPane(theme, "Steps", count, items, cursor, focused, widths[index], height)
		case paneLogs:
			lines = view.logPane(app, job, state, focused, widths[index], height)
		}
		columns = append(columns, block(lines, widths[index], height))
	}

	out := columns[0]
	for index := 1; index < len(columns); index++ {
		border := theme.FaintBorder
		if order[index] == view.focus {
			border = theme.Focus
		}
		out = beside(out, theme.text(border).Render("│"), columns[index])
	}
	return out
}

func paneTitle(theme Theme, title string, focused bool) string {
	style := lipgloss.NewStyle().Padding(0, 1).Foreground(theme.FaintText).Background(theme.SelectedBackground)
	if focused {
		style = lipgloss.NewStyle().Padding(0, 1).Foreground(lipgloss.Color("#0D1117")).Background(theme.Focus).Bold(true)
	}
	return " " + style.Render(title)
}

// listPane draws a pane of two-line items with air between them. The
// selected item carries a rail, brighter in the pane that has the focus.
func listPane(theme Theme, title, count string, items []paneItem, cursor int, focused bool, limit, height int) []string {
	lines := []string{paneTitle(theme, title, focused), "", "  " + theme.faint().Render(count), ""}
	visible := max((height-len(lines)+1)/3, 1)
	first := clamp(cursor-visible/2, 0, max(len(items)-visible, 0))
	for index := first; index < min(first+visible, len(items)); index++ {
		item := items[index]
		name, detail := theme.secondary().Bold(true).Render(item.name), theme.faint().Render(item.detail)
		top := " " + item.icon + " " + truncate(name, limit-5)
		bottom := "   " + truncate(detail, limit-5)
		if index == cursor {
			rail, background := theme.text(theme.FaintText).Render("▌"), theme.Panel
			if focused {
				rail, background = theme.text(theme.Focus).Render("▌"), theme.SelectedBackground
				top = " " + item.icon + " " + truncate(theme.text(theme.Focus).Bold(true).Render(item.name), limit-5)
			}
			top = rail + paintBackground(fit(top, limit-1), background)
			bottom = rail + paintBackground(fit(bottom, limit-1), background)
		} else {
			top, bottom = " "+top, " "+bottom
		}
		lines = append(lines, top, bottom, "")
	}
	return lines
}

func (view *checksView) logPane(app *App, job *gh.Check, state *jobState, focused bool, limit, height int) []string {
	theme := app.theme
	lines := []string{paneTitle(theme, "Job Logs", focused)}
	message := func(text string) []string {
		return append(lines, "", "  "+theme.faint().Render(truncate(text, limit-4)))
	}
	switch {
	case job == nil:
		return lines
	case state == nil:
		// Not an Actions job: show what the check itself reports.
		lines = append(lines, "", "  "+theme.faint().Render("Not an Actions job, so no log. o opens it in the browser."), "")
		for _, line := range app.markdown(job.Summary, limit-4) {
			lines = append(lines, "  "+line)
		}
		return lines
	case job.Running():
		return message("GitHub publishes the log when the job ends. The steps update every 5s.")
	case state.err != nil:
		return message("Could not fetch the log: " + firstLine(state.err))
	case !state.loaded:
		return message(app.spinner() + " loading the log…")
	case len(state.lines) == 0:
		return message("The log is empty.")
	}

	if view.typing {
		lines = append(lines, "  "+theme.text(theme.Focus).Render("/")+view.query+theme.faint().Render("▏"))
	} else if view.query != "" {
		lines = append(lines, "  "+theme.text(theme.Focus).Render("/"+view.query)+theme.faint().Render("  n/N next · esc clear"))
	} else {
		lines = append(lines, "  "+theme.faint().Render("/ search · e/E error · ] [ group"))
	}

	view.ensureRows(app, state, job.JobID)
	bodyHeight := height - 3
	// Keep the cursor line in view, with a little context either side.
	cursorRow, margin := view.rowStart[state.line], min(2, bodyHeight/4)
	state.top = clamp(state.top, cursorRow-bodyHeight+1+margin, cursorRow-margin)
	state.top = clamp(state.top, 0, max(len(view.rows)-bodyHeight, 0))

	digits := len(strconv.Itoa(len(state.lines)))
	thumbSize := clamp(bodyHeight*bodyHeight/max(len(view.rows), 1), 1, bodyHeight)
	thumbStart := 0
	if len(view.rows) > bodyHeight {
		thumbStart = state.top * (bodyHeight - thumbSize) / (len(view.rows) - bodyHeight)
	}
	for offset := 0; offset < bodyHeight; offset++ {
		index := state.top + offset
		scrollbar := theme.text(theme.FaintBorder).Render("░")
		if offset >= thumbStart && offset < thumbStart+thumbSize {
			scrollbar = theme.text(theme.FaintText).Render("█")
		}
		if index >= len(view.rows) {
			lines = append(lines, strings.Repeat(" ", limit-1)+scrollbar)
			continue
		}
		row := view.rows[index]
		number := strings.Repeat(" ", digits)
		if view.rowStart[row.line] == index {
			number = fmt.Sprintf("%*d", digits, row.line+1)
		}
		source := state.lines[row.line]
		marker, numberStyle := " ", theme.text(theme.LineNumber)
		var background = theme.Panel
		painted := false
		switch {
		case source.Kind == gh.LogError:
			marker, background, painted = theme.failure().Render("▐"), theme.RemovedLine, true
		case view.matches(source):
			marker, background, painted = theme.warning().Render("▐"), blend(theme.Note, theme.Panel, 0.75), true
		}
		if row.line == state.line {
			marker, numberStyle = theme.text(theme.Focus).Render("▌"), theme.primary()
			if !painted {
				background, painted = theme.CursorLine, true
			}
		}
		text := fit(" "+numberStyle.Render(number)+"  "+row.text, limit-2)
		if painted {
			text = paintBackground(text, background)
		}
		lines = append(lines, marker+text+scrollbar)
	}
	percent := 100 * (state.line + 1) / len(state.lines)
	position := lipgloss.NewStyle().Padding(0, 1).Foreground(theme.FaintText).Background(theme.SelectedBackground).
		Render(fmt.Sprintf("%d%% (%d/%d)", percent, state.line+1, len(state.lines)))
	return append(lines, " "+position)
}

func (view *checksView) hints() string {
	switch {
	case view.typing:
		return "type to search the log · enter find · esc cancel"
	case view.focus == paneLogs:
		return "j/k line · / search · e/E error · ] [ group · h back · z zoom · esc back"
	}
	return "j/k move · h/l pane · z zoom · f flat · / search · ctrl+r rerun failed · o open · esc back"
}
