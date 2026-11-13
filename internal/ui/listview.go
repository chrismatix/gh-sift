package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/chrismatix/gh-sift/internal/config"
	"github.com/chrismatix/gh-sift/internal/gh"
)

// column is one table column. Width counts the one-cell padding on each
// side, as in gh-dash; zero means "take what is left".
type column struct {
	title string
	width int
	cell  func(row int) []string
}

func hidden(columnConfig config.ColumnConfig) bool {
	return columnConfig.Hidden != nil && *columnConfig.Hidden
}

func columnWidth(columnConfig config.ColumnConfig, fallback int) int {
	if columnConfig.Width != nil {
		return *columnConfig.Width
	}
	return fallback
}

func (list *listScreen) view(app *App, height int) []string {
	theme := app.theme
	lines := list.tabs(app)
	bodyHeight := height - len(lines)
	if !list.previewOpen {
		return append(lines, block(list.table(app, app.width, bodyHeight), app.width, bodyHeight)...)
	}

	preview := app.config.Defaults.Preview
	previewWidth := int(preview.Width)
	if preview.Width < 1 {
		previewWidth = int(preview.Width * float64(app.width))
	}
	// As in gh-dash, "auto" moves the preview below a table that would
	// otherwise be squeezed under 80 columns.
	bottom := preview.Position == "bottom" || (preview.Position != "right" && app.width-previewWidth < 80)
	if list.flipPreview {
		bottom = !bottom
	}

	if bottom {
		previewHeight := int(preview.Height)
		if preview.Height < 1 {
			previewHeight = int(preview.Height * float64(bodyHeight))
		}
		previewHeight = clamp(previewHeight, 5, bodyHeight-9)
		tableHeight := bodyHeight - previewHeight - 1
		content := list.preview(app, app.width-4)
		list.previewScroll = clamp(list.previewScroll, 0, max(len(content)-previewHeight, 0))
		lines = append(lines, block(list.table(app, app.width, tableHeight), app.width, tableHeight)...)
		lines = append(lines, theme.text(theme.PrimaryBorder).Render(strings.Repeat("━", app.width)))
		for _, line := range block(content[list.previewScroll:], app.width-4, previewHeight) {
			lines = append(lines, "  "+line)
		}
		return lines
	}

	mainWidth := app.width - previewWidth
	main := block(list.table(app, mainWidth, bodyHeight), mainWidth, bodyHeight)
	content := list.preview(app, previewWidth-3)
	list.previewScroll = clamp(list.previewScroll, 0, max(len(content)-bodyHeight, 0))
	separator := theme.text(theme.PrimaryBorder).Render("│") + "  "
	return append(lines, beside(main, separator, block(content[list.previewScroll:], previewWidth-3, bodyHeight))...)
}

func (list *listScreen) tabs(app *App) []string {
	theme := app.theme
	showCount := app.config.Theme == nil || app.config.Theme.Ui.SectionsShowCount
	var tabs []string
	for index, current := range list.sections() {
		title := current.title
		if showCount && (current.count() > 0 || !current.loadedAt.IsZero()) {
			title += fmt.Sprintf(" (%s)", shortNumber(current.totalCount))
		}
		// Rows from the cache show at once; the spinner says they are being refreshed.
		if current.loading && current.loadedAt.IsZero() {
			title += " " + app.spinner()
		}
		style := lipgloss.NewStyle().Padding(0, 2).Foreground(theme.FaintText)
		if index == *list.index() {
			style = style.Foreground(theme.PrimaryText).Background(theme.SelectedBackground).Bold(true)
		}
		tabs = append(tabs, style.Render(title))
	}
	row := strings.Join(tabs, theme.text(theme.SecondaryBorder).Render("│"))
	brand := lipgloss.NewStyle().Foreground(theme.Stack).Bold(true).Render("sift") + " "
	return []string{
		spread(truncate(row, app.width-8), brand, app.width),
		theme.text(theme.PrimaryBorder).Render(strings.Repeat("━", app.width)),
	}
}

func (list *listScreen) searchBar(app *App, limit int) []string {
	theme := app.theme
	prefix := "is:pr"
	if list.kind == config.IssuesView {
		prefix = "is:issue"
	}
	current := list.current()
	if repository := list.repositoryFilter(app, current.filters); repository != "" {
		prefix += " " + repository
	}
	border := theme.PrimaryBorder
	content := theme.faint().Render(" "+prefix+" ") + theme.secondary().Render(current.filters)
	if list.searching {
		border = theme.Focus
		content = theme.faint().Render(" "+prefix+" ") + list.search.View()
	}
	return strings.Split(lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Width(limit-2).MaxHeight(3).Render(truncate(content, limit-4)), "\n")
}

func (list *listScreen) table(app *App, limit, height int) []string {
	theme := app.theme
	current := list.current()
	inner := limit - 2
	lines := list.searchBar(app, inner)

	var columns []column
	if list.kind == config.IssuesView {
		columns = list.issueColumns(app, current)
	} else {
		columns = list.pullRequestColumns(app, current)
	}
	fixed := 0
	for _, each := range columns {
		fixed += each.width
	}
	// When the fixed columns leave the title no room, the rightmost ones go.
	for inner-fixed < 24 && len(columns) > 3 && columns[len(columns)-1].width > 0 {
		fixed -= columns[len(columns)-1].width
		columns = columns[:len(columns)-1]
	}
	for index := range columns {
		if columns[index].width == 0 {
			columns[index].width = max(inner-fixed, 10)
		}
	}

	header := ""
	for _, each := range columns {
		header += " " + theme.bold().Render(fit(each.title, each.width-2)) + " "
	}
	lines = append(lines, header, "")

	compact := app.config.Theme != nil && app.config.Theme.Ui.Table.Compact
	separator := app.config.Theme == nil || app.config.Theme.Ui.Table.ShowSeparator
	rowHeight := 2
	if compact {
		rowHeight = 1
	}
	if separator {
		rowHeight++
	}
	bodyHeight := height - len(lines)
	visible := max(bodyHeight/rowHeight, 1)
	current.top = clamp(current.top, max(current.cursor-visible+1, 0), current.cursor)

	switch {
	case current.err != nil:
		lines = append(lines, "", "  "+theme.failure().Render(truncate(firstLine(current.err), inner-4)))
	case current.count() == 0 && current.loading:
		lines = append(lines, "", "  "+theme.faint().Render(app.spinner()+" Fetching…"))
	case current.count() == 0:
		lines = append(lines, "", "  "+theme.faint().Render("Nothing matches these filters"))
	}

	// Rows of the stack under the cursor get a rail, so the stack reads as a group.
	selectedStack := ""
	if row := current.selectedPullRequest(); row != nil && row.Stack != nil {
		selectedStack = row.Stack.ID
	}
	for row := current.top; row < min(current.top+visible, current.count()); row++ {
		rail := " "
		if selectedStack != "" && row < len(current.pullRequests) && current.pullRequests[row].Stack != nil && current.pullRequests[row].Stack.ID == selectedStack {
			rail = theme.stack().Render("▎")
		}
		cells := make([][]string, len(columns))
		for index, each := range columns {
			cells[index] = each.cell(row)
		}
		for lineIndex := 0; lineIndex < rowHeight-map[bool]int{true: 1, false: 0}[separator]; lineIndex++ {
			line := ""
			for index, each := range columns {
				text := ""
				if lineIndex < len(cells[index]) {
					text = cells[index][lineIndex]
				}
				padding := " "
				if index == 0 {
					padding = rail
				}
				line += padding + fit(text, each.width-2) + " "
			}
			if row == current.cursor {
				line = paintBackground(fit(line, inner), theme.SelectedBackground)
			}
			lines = append(lines, line)
		}
		if separator {
			lines = append(lines, theme.text(theme.FaintBorder).Render(strings.Repeat("─", inner)))
		}
	}
	for index := range lines {
		lines[index] = " " + lines[index]
	}
	return lines
}

func (list *listScreen) pullRequestColumns(app *App, current *section) []column {
	theme := app.theme
	layout := current.prLayout
	rows := current.pullRequests
	compact := app.config.Theme != nil && app.config.Theme.Ui.Table.Compact
	single := func(render func(row gh.PullRequestRow) string) func(int) []string {
		return func(index int) []string { return []string{render(rows[index])} }
	}

	var columns []column
	if !hidden(layout.State) {
		columns = append(columns, column{"", 3, single(func(row gh.PullRequestRow) string {
			return theme.stateGlyph(row.State, row.IsDraft, row.IsInMergeQueue)
		})})
	}
	if compact && !hidden(layout.Repo) {
		columns = append(columns, column{glyphRepo, columnWidth(layout.Repo, 20), single(func(row gh.PullRequestRow) string {
			_, name, _ := strings.Cut(row.Repository, "/")
			return theme.faint().Render(name)
		})})
	}
	columns = append(columns, column{"Title", columnWidth(layout.Title, 0), func(index int) []string {
		row := rows[index]
		number := theme.secondary()
		if row.State != "OPEN" {
			number = theme.faint()
		}
		if compact {
			return []string{number.Render(fmt.Sprintf("#%d ", row.Number)) + theme.bold().Render(row.Title)}
		}
		first := theme.secondary().Render(fmt.Sprintf("%s #%d by ", row.Repository, row.Number)) + theme.secondary().Bold(true).Render("@"+row.Author)
		// gh-dash ties the branch on this line to the base column's visibility.
		if !hidden(layout.Base) {
			first += theme.secondary().Render(" · " + row.HeadRefName)
		}
		return []string{first, theme.bold().Render(row.Title)}
	}})
	if compact && !hidden(layout.Author) {
		columns = append(columns, column{"Author", columnWidth(layout.Author, 15), single(func(row gh.PullRequestRow) string {
			return theme.secondary().Render(row.Author)
		})})
	}
	columns = append(columns, column{glyphStack, 7, single(func(row gh.PullRequestRow) string {
		return theme.stack().Render(stackBadge(row.StackPosition, row.Stack))
	})})
	if !hidden(layout.Base) {
		columns = append(columns, column{"Base", columnWidth(layout.Base, 15), single(func(row gh.PullRequestRow) string {
			return theme.secondary().Render(row.BaseRefName)
		})})
	}
	if !hidden(layout.NumComments) {
		columns = append(columns, column{glyphComments, columnWidth(layout.NumComments, 4), single(func(row gh.PullRequestRow) string {
			if row.CommentCount == 0 {
				return ""
			}
			return theme.faint().Render(shortNumber(row.CommentCount))
		})})
	}
	if !hidden(layout.ReviewStatus) {
		columns = append(columns, column{glyphReview, columnWidth(layout.ReviewStatus, 4), single(func(row gh.PullRequestRow) string {
			return theme.reviewGlyph(row.ReviewDecision, row.ReviewCount)
		})})
	}
	if !hidden(layout.Ci) {
		columns = append(columns, column{glyphCI, columnWidth(layout.Ci, 4), single(func(row gh.PullRequestRow) string {
			return theme.checksGlyph(row.ChecksState)
		})})
	}
	if !hidden(layout.Lines) {
		columns = append(columns, column{glyphLines, columnWidth(layout.Lines, 15), single(func(row gh.PullRequestRow) string {
			return theme.success().Render("+"+shortNumber(row.Additions)) + " " + theme.failure().Render("-"+shortNumber(row.Deletions))
		})})
	}
	if !hidden(layout.UpdatedAt) {
		columns = append(columns, column{glyphUpdated, columnWidth(layout.UpdatedAt, 5), single(func(row gh.PullRequestRow) string {
			return theme.faint().Render(elapsed(row.UpdatedAt))
		})})
	}
	if !hidden(layout.CreatedAt) {
		columns = append(columns, column{glyphCreated, columnWidth(layout.CreatedAt, 5), single(func(row gh.PullRequestRow) string {
			return theme.faint().Render(elapsed(row.CreatedAt))
		})})
	}
	return columns
}

func (list *listScreen) issueColumns(app *App, current *section) []column {
	theme := app.theme
	layout := current.issueLayout
	rows := current.issues
	single := func(render func(row gh.IssueRow) string) func(int) []string {
		return func(index int) []string { return []string{render(rows[index])} }
	}
	columns := []column{
		{"", 3, single(func(row gh.IssueRow) string {
			if row.State == "OPEN" {
				return theme.text(theme.Open).Render(glyphOpenIssue)
			}
			return theme.text(theme.Merged).Render(glyphClosedIssue)
		})},
		{"Title", 0, func(index int) []string {
			row := rows[index]
			return []string{
				theme.secondary().Render(fmt.Sprintf("%s #%d by @%s", row.Repository, row.Number, row.Author)),
				theme.bold().Render(row.Title),
			}
		}},
	}
	if !hidden(layout.Comments) {
		columns = append(columns, column{glyphComments, columnWidth(layout.Comments, 6), single(func(row gh.IssueRow) string {
			return theme.faint().Render(shortNumber(row.CommentCount))
		})})
	}
	if !hidden(layout.Reactions) {
		columns = append(columns, column{glyphReactions, columnWidth(layout.Reactions, 6), single(func(row gh.IssueRow) string {
			return theme.faint().Render(shortNumber(row.ReactionCount))
		})})
	}
	if !hidden(layout.UpdatedAt) {
		columns = append(columns, column{glyphUpdated, columnWidth(layout.UpdatedAt, 5), single(func(row gh.IssueRow) string {
			return theme.faint().Render(elapsed(row.UpdatedAt))
		})})
	}
	if !hidden(layout.CreatedAt) {
		columns = append(columns, column{glyphCreated, columnWidth(layout.CreatedAt, 5), single(func(row gh.IssueRow) string {
			return theme.faint().Render(elapsed(row.CreatedAt))
		})})
	}
	return columns
}

// preview is the sidebar: a summary of the selected row, enough to decide
// whether to open it.
func (list *listScreen) preview(app *App, limit int) []string {
	theme := app.theme
	current := list.current()
	if issue := current.selectedIssue(); list.kind == config.IssuesView && issue != nil {
		lines := []string{"", theme.faint().Render(fmt.Sprintf("%s · #%d", issue.Repository, issue.Number))}
		for _, line := range wrap(issue.Title, limit) {
			lines = append(lines, theme.bold().Render(line))
		}
		lines = append(lines, "", theme.faint().Render(fmt.Sprintf("by @%s · updated %s · %s", issue.Author, ago(issue.UpdatedAt), plural(issue.CommentCount, "comment"))))
		if labels := labelLine(theme, issue.Labels); labels != "" {
			lines = append(lines, "", labels)
		}
		return append(append(lines, ""), app.markdown(issue.Body, limit)...)
	}
	row := current.selectedPullRequest()
	if row == nil {
		return nil
	}
	detail := app.data(pullRequestKey{row.Repository, row.Number}).detail
	lines := []string{"", theme.faint().Render(fmt.Sprintf("%s · #%d", row.Repository, row.Number))}
	for _, line := range wrap(row.Title, limit) {
		lines = append(lines, theme.bold().Render(line))
	}
	lines = append(lines, "",
		theme.stateLabel(row.State, row.IsDraft, row.IsInMergeQueue)+"  "+theme.secondary().Render(row.HeadRefName)+theme.faint().Render(" → ")+theme.secondary().Render(row.BaseRefName),
		theme.faint().Render(fmt.Sprintf("by @%s · updated %s · ", row.Author, ago(row.UpdatedAt)))+
			theme.success().Render(fmt.Sprintf("+%d", row.Additions))+" "+theme.failure().Render(fmt.Sprintf("-%d", row.Deletions)),
	)
	if row.Stack != nil {
		stack := row.Stack
		if detail != nil && detail.Stack != nil {
			stack = detail.Stack
		}
		lines = append(lines, "", theme.stack().Bold(true).Render(fmt.Sprintf("Stack · %d of %d", row.StackPosition, stack.Size)))
		lines = append(lines, stackLines(theme, stack, row.StackPosition, limit)...)
	}
	if detail == nil {
		return append(lines, "", theme.faint().Render(app.spinner()+" loading…"))
	}
	lines = append(lines, "")
	lines = append(lines, statusLines(app, detail, limit)...)
	if body := app.markdown(detail.Body, limit); len(body) > 0 {
		lines = append(lines, "", theme.text(theme.FaintBorder).Render(strings.Repeat("─", limit)), "")
		lines = append(lines, body...)
	}
	return lines
}

func labelLine(theme Theme, labels []gh.Label) string {
	var parts []string
	for _, label := range labels {
		parts = append(parts, theme.text(lipgloss.Color("#"+label.Color)).Render("● ")+theme.secondary().Render(label.Name))
	}
	return strings.Join(parts, "  ")
}

// statusLines answers "can this merge?": review, checks, conflicts, people.
func statusLines(app *App, detail *gh.PullRequest, limit int) []string {
	theme := app.theme
	label := func(name string) string { return theme.faint().Render(fit(name, 10)) }

	review := theme.warning().Render("review required")
	switch detail.ReviewDecision {
	case "APPROVED":
		review = theme.success().Render("approved")
	case "CHANGES_REQUESTED":
		review = theme.failure().Render("changes requested")
	case "":
		review = theme.faint().Render("no review needed")
	}
	if unresolved := detail.UnresolvedThreads(); unresolved > 0 {
		review += theme.faint().Render(" · ") + theme.warning().Render(plural(unresolved, "open thread"))
	}
	lines := []string{label("Review") + review}

	var people []string
	for _, each := range detail.LatestReviews {
		mark := theme.faint().Render("●")
		switch each.State {
		case "APPROVED":
			mark = theme.success().Render("✓")
		case "CHANGES_REQUESTED":
			mark = theme.failure().Render("✗")
		}
		people = append(people, mark+" "+theme.secondary().Render(each.Author))
	}
	for _, requested := range detail.ReviewRequests {
		people = append(people, theme.warning().Render("○")+" "+theme.secondary().Render(requested))
	}
	if len(people) > 0 {
		lines = append(lines, label("Reviewers")+truncate(strings.Join(people, "  "), limit-10))
	}

	lines = append(lines, label("Checks")+checksSummary(theme, detail.Checks))

	merge := ""
	switch {
	case detail.State != "OPEN":
	case detail.IsInMergeQueue:
		merge = theme.warning().Render("in the merge queue")
	case detail.Mergeable == "CONFLICTING":
		merge = theme.failure().Render("conflicts with " + detail.BaseRefName)
	case detail.MergeStateStatus == "CLEAN":
		merge = theme.success().Render("ready to merge")
	case detail.MergeStateStatus == "BEHIND":
		merge = theme.warning().Render("behind " + detail.BaseRefName)
	case detail.MergeStateStatus != "":
		merge = theme.warning().Render(strings.ToLower(detail.MergeStateStatus))
	}
	if detail.AutoMergeMethod != "" {
		merge += theme.faint().Render(" · auto-merge on")
	}
	if merge != "" {
		lines = append(lines, label("Merge")+merge)
	}
	if labels := labelLine(theme, detail.Labels); labels != "" {
		lines = append(lines, label("Labels")+truncate(labels, limit-10))
	}
	if len(detail.Assignees) > 0 {
		lines = append(lines, label("Assignees")+theme.secondary().Render(strings.Join(detail.Assignees, ", ")))
	}
	return lines
}

func checksSummary(theme Theme, checks []gh.Check) string {
	if len(checks) == 0 {
		return theme.faint().Render("none")
	}
	passed, failed, running := 0, 0, 0
	for _, check := range checks {
		switch {
		case check.Running():
			running++
		case check.Failed():
			failed++
		default:
			passed++
		}
	}
	parts := []string{theme.success().Render(fmt.Sprintf("✓ %d", passed))}
	if failed > 0 {
		parts = append(parts, theme.failure().Render(fmt.Sprintf("✗ %d", failed)))
	}
	if running > 0 {
		parts = append(parts, theme.warning().Render(fmt.Sprintf("● %d", running)))
	}
	return strings.Join(parts, "  ")
}
