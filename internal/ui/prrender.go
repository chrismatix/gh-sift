package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/chrismatix/gh-sift/internal/gh"
)

func (screen *pullRequestScreen) view(app *App, height int) []string {
	theme := app.theme
	row := screen.row(app)
	data := app.data(screen.key)

	var lines []string
	if screen.tab == tabFiles {
		// The diff gets the room: the PR shrinks to the right end of the tab row.
		identity := ""
		if row != nil {
			identity = theme.faint().Render(fmt.Sprintf("#%d ", row.Number)) + theme.secondary().Render(truncate(row.Title, max(app.width-70, 10)))
			if row.Stack != nil {
				identity += "  " + theme.stack().Render(glyphStack+" "+stackBadge(row.StackPosition, row.Stack))
			}
		}
		lines = append(lines, spread(screen.tabRow(app), identity+" ", app.width))
	} else {
		lines = append(lines, screen.header(app, row)...)
		lines = append(lines, "", screen.tabRow(app))
	}
	lines = append(lines, theme.text(theme.PrimaryBorder).Render(strings.Repeat("━", app.width)))
	bodyHeight := height - len(lines)

	switch {
	case data.detail == nil && data.err != nil:
		return append(lines, "", "  "+theme.failure().Render(firstLine(data.err)))
	case data.detail == nil:
		return append(lines, "", "  "+theme.faint().Render(app.spinner()+" Loading the pull request…"))
	}
	switch screen.tab {
	case tabChecks:
		lines = append(lines, screen.checks.view(app, bodyHeight)...)
	case tabThreads:
		lines = append(lines, screen.threadsView(app, bodyHeight)...)
	case tabFiles:
		lines = append(lines, screen.diff.view(app, bodyHeight)...)
	default:
		lines = append(lines, screen.overviewView(app, bodyHeight)...)
	}
	return lines
}

func (screen *pullRequestScreen) header(app *App, row *gh.PullRequestRow) []string {
	theme := app.theme
	if row == nil {
		return []string{"", "  " + theme.faint().Render(fmt.Sprintf("%s #%d", screen.key.repository, screen.key.number)), "", ""}
	}
	stack := ""
	if row.Stack != nil {
		stack = theme.stack().Bold(true).Render(fmt.Sprintf("%s stack %s", glyphStack, stackBadge(row.StackPosition, row.Stack))) + "  "
	}
	identity := "  " + theme.faint().Render(fmt.Sprintf("%s #%d", row.Repository, row.Number))
	facts := "  " + theme.stateLabel(row.State, row.IsDraft, row.IsInMergeQueue) + "  " +
		theme.secondary().Bold(true).Render("@"+row.Author) + theme.faint().Render(" wants to merge ") +
		theme.secondary().Render(row.HeadRefName) + theme.faint().Render(" → ") + theme.secondary().Render(row.BaseRefName)
	numbers := ""
	// A seed row from a stack entry has no author or sizes yet.
	if row.Author == "" {
		facts = "  " + theme.stateLabel(row.State, row.IsDraft, row.IsInMergeQueue)
	} else {
		numbers = theme.success().Render(fmt.Sprintf("+%d", row.Additions)) + " " + theme.failure().Render(fmt.Sprintf("-%d", row.Deletions)) +
			theme.faint().Render(" · updated "+ago(row.UpdatedAt)) + "  "
	}
	return []string{
		"",
		spread(identity, stack, app.width),
		"  " + theme.bold().Render(truncate(row.Title, app.width-4)),
		spread(facts, numbers, app.width),
	}
}

func (screen *pullRequestScreen) tabRow(app *App) string {
	theme := app.theme
	detail := screen.detail(app)
	labels := []string{"Overview", "Checks", "Threads", "Files"}
	if detail != nil {
		if len(detail.Checks) > 0 {
			labels[1] += " " + checksSummary(theme, detail.Checks)
		}
		if unresolved := detail.UnresolvedThreads(); unresolved > 0 {
			labels[2] += " " + theme.warning().Render(fmt.Sprint(unresolved))
		}
		labels[3] += " " + theme.faint().Render(fmt.Sprint(detail.ChangedFiles))
		if pending := detail.PendingComments(); pending > 0 {
			labels[3] += " " + theme.warning().Render(fmt.Sprintf("● %d pending", pending))
		}
	}
	row := " "
	for index, label := range labels {
		style := lipgloss.NewStyle().Padding(0, 2).Foreground(theme.FaintText)
		if pullRequestTab(index) == screen.tab {
			// The background must survive the coloured counts inside the label.
			row += paintBackground(lipgloss.NewStyle().Padding(0, 2).Foreground(theme.PrimaryText).Bold(true).Render(label), theme.SelectedBackground)
			continue
		}
		row += style.Render(label)
	}
	return row
}

// overviewView is the description and the conversation in one scroll, with
// the merge status and the stack in a rail beside it.
func (screen *pullRequestScreen) overviewView(app *App, height int) []string {
	theme := app.theme
	detail := screen.detail(app)
	railWidth := 0
	if app.width >= 110 {
		railWidth = clamp(app.width*2/5, 44, 64)
	}
	mainWidth := app.width - railWidth - 4
	if railWidth > 0 {
		mainWidth -= 3
	}

	if screen.overviewLines == nil || screen.overviewWidth != mainWidth {
		screen.overviewWidth = mainWidth
		var lines []string
		if railWidth == 0 {
			lines = append(lines, screen.rail(app, mainWidth)...)
			lines = append(lines, "", theme.text(theme.FaintBorder).Render(strings.Repeat("─", mainWidth)))
		}
		lines = append(lines, "")
		if body := app.markdown(detail.Body, min(mainWidth, 100)); len(body) > 0 {
			lines = append(lines, body...)
		} else {
			lines = append(lines, theme.faint().Render("No description."))
		}
		lines = append(lines, "", theme.text(theme.FaintBorder).Render(strings.Repeat("─", mainWidth)), "")
		lines = append(lines, screen.timeline(app, min(mainWidth, 100))...)
		screen.overviewLines = append(lines, "")
	}

	screen.overviewScroll = clamp(screen.overviewScroll, 0, max(len(screen.overviewLines)-height, 0))
	main := block(screen.overviewLines[screen.overviewScroll:], mainWidth, height)
	for index := range main {
		main[index] = "  " + main[index] + "  "
	}
	if railWidth == 0 {
		return main
	}
	rail := block(append([]string{""}, screen.rail(app, railWidth-2)...), railWidth, height)
	return beside(main, theme.text(theme.FaintBorder).Render("│")+"  ", rail)
}

func (screen *pullRequestScreen) rail(app *App, limit int) []string {
	theme := app.theme
	detail := screen.detail(app)
	lines := statusLines(app, detail, limit)
	if detail.Stack != nil {
		lines = append(lines, "", theme.stack().Bold(true).Render(fmt.Sprintf("Stack · %d of %d", detail.StackPosition, detail.Stack.Size)), "")
		lines = append(lines, stackLines(theme, detail.Stack, detail.StackPosition, limit)...)
		lines = append(lines, "", theme.faint().Render("K up the stack · J down · m merge"))
	}
	return lines
}

func (screen *pullRequestScreen) timeline(app *App, limit int) []string {
	theme := app.theme
	var lines []string
	previous := ""
	for _, item := range screen.detail(app).Timeline {
		// Runs of commits and events stay tight; anything with a body gets air.
		if previous != "" && !(item.Kind == previous && item.Kind == "commit") {
			lines = append(lines, "")
		}
		previous = item.Kind
		who := theme.secondary().Bold(true).Render("@" + item.Author)
		when := theme.faint().Render(" · " + ago(item.CreatedAt))
		switch item.Kind {
		case "commit":
			lines = append(lines, theme.faint().Render("◦ "+item.Author+"  ")+theme.secondary().Render(truncate(item.Body, limit-12))+when)
			continue
		case "force-push":
			lines = append(lines, theme.warning().Render("↻ ")+who+theme.faint().Render(" force-pushed")+when)
			continue
		case "merged":
			lines = append(lines, theme.text(theme.Merged).Render(glyphMergedPR+" ")+who+theme.text(theme.Merged).Render(" merged")+when)
			continue
		case "closed":
			lines = append(lines, theme.text(theme.Closed).Render(glyphClosedPR+" ")+who+theme.faint().Render(" closed this")+when)
			continue
		case "reopened":
			lines = append(lines, theme.text(theme.Open).Render(glyphOpenPR+" ")+who+theme.faint().Render(" reopened this")+when)
			continue
		case "review-requested":
			lines = append(lines, theme.faint().Render("○ ")+who+theme.faint().Render(" requested review from ")+theme.secondary().Render("@"+item.Body)+when)
			continue
		}
		mark, verb, bar := theme.text(theme.Focus).Render("● "), " commented", theme.FaintText
		if item.Kind == "review" {
			switch item.State {
			case "APPROVED":
				mark, verb, bar = theme.success().Render("✓ "), theme.success().Render(" approved"), theme.SuccessText
			case "CHANGES_REQUESTED":
				mark, verb, bar = theme.failure().Render("✗ "), theme.failure().Render(" requested changes"), theme.ErrorText
			default:
				verb = " reviewed"
			}
		}
		lines = append(lines, mark+who+theme.faint().Render(verb)+when)
		for _, line := range app.markdown(item.Body, limit-2) {
			lines = append(lines, theme.text(bar).Render("│ ")+line)
		}
	}
	if len(lines) == 0 {
		return []string{theme.faint().Render("No conversation yet.")}
	}
	return lines
}

func (screen *pullRequestScreen) threadsView(app *App, height int) []string {
	theme := app.theme
	threads := screen.threads(app)
	if len(threads) == 0 {
		message := "No review threads."
		if screen.hideResolved && len(screen.detail(app).Threads) > 0 {
			message = "No open threads. h shows the resolved ones."
		}
		return []string{"", "  " + theme.faint().Render(message)}
	}
	listWidth := clamp(app.width*2/5, 40, 80)
	detailWidth := app.width - listWidth - 5

	// Two lines and a rule per thread, like the PR table.
	visible := max((height-1)/3, 1)
	first := clamp(screen.threadCursor-visible/2, 0, max(len(threads)-visible, 0))
	list := []string{""}
	for index := first; index < min(first+visible, len(threads)); index++ {
		thread := threads[index]
		mark := theme.warning().Render("○")
		if thread.IsResolved {
			mark = theme.success().Render("✓")
		}
		tags := ""
		if len(thread.Comments) > 0 {
			tags = severityTag(theme, severity(thread.Comments[0].Body))
		}
		if thread.IsOutdated {
			tags += theme.faint().Render(" outdated")
		}
		about, people := "", ""
		if len(thread.Comments) > 0 {
			about = summary(thread.Comments[0].Body)
			people = "@" + thread.Comments[0].Author
			if replies := len(thread.Comments) - 1; replies > 0 {
				people += " · " + plural(replies, "reply")
				if replies > 1 {
					people = strings.Replace(people, "replys", "replies", 1)
				}
			}
			// The latest comment, so a fresh reply on an old thread reads as recent.
			if when := elapsed(thread.Comments[len(thread.Comments)-1].CreatedAt); when != "" {
				people += " · " + when
			}
		}
		// The file name and line matter most, so the directory gives way first.
		location := fmt.Sprintf("%s:%d", shortPath(thread.Path, listWidth-width(people)-width(tags)-12), max(thread.Line, thread.OriginalLine))
		top := spread(" "+mark+" "+theme.secondary().Render(location)+tags, theme.faint().Render(people)+" ", listWidth)
		bottom := fit("   "+theme.bold().Render(truncate(about, listWidth-5)), listWidth)
		if thread.IsResolved {
			bottom = fit("   "+theme.faint().Render(truncate(about, listWidth-5)), listWidth)
		}
		if index == screen.threadCursor {
			top, bottom = paintBackground(top, theme.SelectedBackground), paintBackground(bottom, theme.SelectedBackground)
		}
		list = append(list, top, bottom, theme.text(theme.FaintBorder).Render(strings.Repeat("─", listWidth)))
	}

	detail := append([]string{""}, threadDetail(app, threads[screen.threadCursor], detailWidth)...)
	screen.threadScroll = clamp(screen.threadScroll, 0, max(len(detail)-height, 0))
	return beside(block(list, listWidth, height), " "+theme.text(theme.FaintBorder).Render("│")+"  ", block(detail[screen.threadScroll:], detailWidth, height))
}

func severityTag(theme Theme, level string) string {
	switch level {
	case "":
		return ""
	case "critical":
		return " " + theme.failure().Bold(true).Render(level)
	case "major":
		return " " + theme.warning().Bold(true).Render(level)
	}
	return " " + theme.faint().Render(level)
}

// shortPath keeps the file name and as much of its directory as fits.
func shortPath(path string, limit int) string {
	if width(path) <= limit {
		return path
	}
	slash := strings.LastIndex(path, "/")
	if slash < 0 {
		return truncate(path, limit)
	}
	name := path[slash:]
	room := limit - width(name) - 1
	if room < 4 {
		return "…" + name
	}
	directory := path[:slash]
	return "…" + directory[len(directory)-min(room, len(directory)):] + name
}

// threadDetail reads like a chat: status, the code it is about, the comments.
func threadDetail(app *App, thread gh.Thread, limit int) []string {
	theme := app.theme
	banner := lipgloss.NewStyle().Foreground(lipgloss.Color("#0D1117")).Background(theme.WarningText).Bold(true).Padding(0, 1).Render("OPEN")
	if thread.IsResolved {
		banner = lipgloss.NewStyle().Foreground(lipgloss.Color("#0D1117")).Background(theme.SuccessText).Bold(true).Padding(0, 1).Render("RESOLVED")
		if thread.ResolvedBy != "" {
			banner += theme.faint().Render("  by @" + thread.ResolvedBy)
		}
	}
	if thread.IsOutdated {
		banner += theme.faint().Render("  outdated")
	}
	lines := []string{banner, "", theme.secondary().Render(fmt.Sprintf("%s:%d", thread.Path, max(thread.Line, thread.OriginalLine))), ""}

	hunk := strings.Split(thread.DiffHunk, "\n")
	for _, code := range hunk[max(len(hunk)-5, 0):] {
		if strings.HasPrefix(code, "@@") || code == "" {
			continue
		}
		style := theme.faint()
		switch code[0] {
		case '+':
			style = theme.text(theme.AddedSign)
		case '-':
			style = theme.text(theme.RemovedSign)
		}
		lines = append(lines, theme.text(theme.FaintBorder).Render("▏ ")+style.Render(truncate(strings.ReplaceAll(code, "\t", "    "), limit-2)))
	}
	return append(lines, threadComments(app, thread, limit)...)
}

func threadComments(app *App, thread gh.Thread, limit int) []string {
	theme := app.theme
	var lines []string
	for index, comment := range thread.Comments {
		mark := theme.text(theme.Focus).Render("● ")
		if index > 0 {
			mark = theme.faint().Render("↳ ")
		}
		heading := mark + theme.secondary().Bold(true).Render("@"+comment.Author) + theme.faint().Render(" · "+ago(comment.CreatedAt))
		if comment.Pending {
			heading += "  " + theme.warning().Render("pending")
		}
		lines = append(lines, "", heading)
		for _, line := range app.markdown(comment.Body, limit-2) {
			lines = append(lines, theme.text(theme.FaintBorder).Render("│ ")+line)
		}
	}
	return lines
}
