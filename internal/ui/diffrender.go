package ui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chrismatix/gh-sift/internal/diff"
)

// paint is the hot path's cheap alternative to a lipgloss style.
func paint(text string, foreground, background color.Color, bold bool) string {
	style := ansi.Style{}
	if foreground != nil {
		style = style.ForegroundColor(foreground)
	}
	if background != nil {
		style = style.BackgroundColor(background)
	}
	if bold {
		style = style.Bold()
	}
	if len(style) == 0 {
		return text
	}
	return style.Styled(text)
}

func blend(from, to color.Color, amount float64) color.Color {
	fromRed, fromGreen, fromBlue, _ := from.RGBA()
	toRed, toGreen, toBlue, _ := to.RGBA()
	mix := func(first, second uint32) uint8 {
		return uint8((float64(first)*(1-amount) + float64(second)*amount) / 257)
	}
	return color.RGBA{mix(fromRed, toRed), mix(fromGreen, toGreen), mix(fromBlue, toBlue), 255}
}

var tokenColors = map[string]color.Color{}

func tokenColor(hex string) color.Color {
	if hex == "" {
		return nil
	}
	if cached, ok := tokenColors[hex]; ok {
		return cached
	}
	tokenColors[hex] = lipgloss.Color(hex)
	return tokenColors[hex]
}

func (view *diffView) view(app *App, height int) []string {
	theme := app.theme
	data := app.data(app.pullRequest.key)
	switch {
	case data.files == nil && data.diffErr != nil:
		return []string{"", "  " + theme.failure().Render(firstLine(data.diffErr))}
	case data.files == nil:
		return []string{"", "  " + theme.faint().Render(app.spinner()+" Loading the diff…")}
	case len(view.files) == 0:
		return []string{"", "  " + theme.faint().Render("This pull request changes no files.")}
	}
	view.ensureRows(app)
	lastTop := max(len(view.rows)-height, 0)
	// While the composer covers the bottom of the pane, the end of the diff
	// may scroll up past it, or the last lines could not be commented on in view.
	if app.overlay != nil && app.overlay.kind == overlayComposer {
		lastTop = max(len(view.rows)-1, 0)
	}
	view.top = clamp(view.top, 0, lastTop)

	paneWidth := view.paneWidth(app)
	// Files run into each other, so the cursor's file gets an inverted header and a light frame.
	currentFile := -1
	if view.cursor >= 0 && view.cursor < len(view.rows) {
		currentFile = view.rows[view.cursor].file
	}
	frameTone := theme.SecondaryBorder
	pane := make([]string, height)
	for line := range pane {
		index := view.top + line
		if index >= len(view.rows) {
			pane[line] = strings.Repeat(" ", paneWidth)
			continue
		}
		row := view.rows[index]
		// The file header stays pinned while its file scrolls under it.
		if line == 0 && row.kind != rowFile && row.kind != rowBlank {
			pane[line] = view.fileHeader(app, row.file, paneWidth, false, row.file == currentFile)
			continue
		}
		switch {
		case row.kind == rowFile:
			pane[line] = view.fileHeader(app, row.file, paneWidth, index == view.cursor, row.file == currentFile)
		case row.kind == rowBlank && row.file == currentFile+1 && currentFile >= 0:
			pane[line] = paint("╰"+strings.Repeat("─", max(paneWidth-2, 0))+"╯", frameTone, nil, false)
		case row.kind != rowBlank && row.file == currentFile:
			pane[line] = paint("│", frameTone, nil, false) + view.renderRow(app, index, paneWidth-2) + paint("│", frameTone, nil, false)
		default:
			pane[line] = " " + view.renderRow(app, index, paneWidth-2) + " "
		}
	}
	sidebarWidth := view.sidebarWidth(app)
	if sidebarWidth == 0 {
		return pane
	}
	return beside(block(view.sidebar(app, sidebarWidth, height), sidebarWidth, height), theme.text(theme.FaintBorder).Render("│"), pane)
}

func (view *diffView) renderRow(app *App, index, limit int) string {
	theme := app.theme
	row := view.rows[index]
	onCursor := index == view.cursor
	current := view.rows[view.cursor]
	inCurrentHunk := row.file == current.file && row.hunk == current.hunk && row.hunk >= 0

	rail := func(tone color.Color) string {
		if !inCurrentHunk {
			tone = blend(tone, theme.Panel, 0.6)
		}
		if onCursor || view.selected(index) {
			tone = theme.Focus
		}
		return paint("▌", tone, nil, false)
	}
	highlight := func(line string) string {
		if onCursor {
			return paintBackground(fit(line, limit), theme.CursorLine)
		}
		return fit(line, limit)
	}

	switch row.kind {
	case rowBlank:
		return strings.Repeat(" ", limit)
	case rowHunk, rowFold:
		file := view.files[row.file]
		if row.hunk < 0 {
			return highlight(rail(theme.LineNumber) + "   " + theme.faint().Render(row.text))
		}
		hunk := file.Hunks[row.hunk]
		header := fmt.Sprintf("@@ -%d,%d +%d,%d @@", hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
		section := strings.ReplaceAll(hunk.Section, "\t", " ")
		if row.kind == rowFold {
			return highlight(rail(theme.LineNumber) + " " + theme.success().Render("✓ read") + theme.faint().Render(" · "+header+" "+truncate(section, 40)+" · "+plural(len(hunk.Lines), "line")))
		}
		line := rail(theme.LineNumber) + " " + theme.text(theme.LineNumber).Render(header) + " " + theme.secondary().Render(section)
		if view.hunkRead(app, file, hunk) {
			line += theme.success().Render("  ✓ read")
		}
		return highlight(line)
	case rowThread:
		return view.threadRow(app, row, index, limit)
	}
	return view.codeRow(app, row, index, limit, rail)
}

func (view *diffView) fileHeader(app *App, fileIndex, limit int, onCursor, current bool) string {
	theme := app.theme
	file := view.files[fileIndex]
	status := map[diff.Status]color.Color{diff.AddedFile: theme.AddedSign, diff.DeletedFile: theme.RemovedSign, diff.Renamed: theme.Note, diff.Modified: theme.Note}[file.Status]
	path := file.Path()
	directory, name := "", path
	if slash := strings.LastIndex(path, "/"); slash >= 0 {
		directory, name = path[:slash+1], path[slash+1:]
	}
	left := " " + paint(string(file.Status), status, nil, true) + "  " + theme.faint().Render(directory) + theme.bold().Render(name)
	if file.Status == diff.Renamed && file.OldPath != file.NewPath {
		left += theme.faint().Render("  ← " + file.OldPath)
	}
	right := ""
	switch view.viewed[path] {
	case "VIEWED":
		right = theme.success().Render("✓ viewed") + "  "
	case "DISMISSED":
		right = theme.warning().Render("↻ changed since viewed") + "  "
	}
	if open := threadCount(view.threads[path]); open > 0 {
		right += theme.text(theme.Note).Render(fmt.Sprintf("● %d", open)) + "  "
	}
	right += theme.success().Render(fmt.Sprintf("+%d", file.Additions)) + " " + theme.failure().Render(fmt.Sprintf("-%d", file.Deletions)) + " "
	marker := " "
	if onCursor {
		marker = paint("▌", theme.Focus, nil, false)
	}
	if current {
		// Inverted: one solid bar, so the colours of the parts give way to it.
		bar := ansi.Strip(left)
		marker = paint(" ", nil, theme.PrimaryText, false)
		if onCursor {
			marker = paint("▌", theme.Focus, theme.PrimaryText, false)
		}
		return marker + paint(spread(truncate(bar, limit-width(right)-3), ansi.Strip(right), limit-1), theme.InvertedText, theme.PrimaryText, true)
	}
	return paintBackground(marker+spread(truncate(left, limit-width(right)-3), right, limit-1), theme.Panel)
}

func (view *diffView) threadRow(app *App, row diffRow, index, limit int) string {
	theme := app.theme
	border := theme.Note
	if row.thread.IsResolved || row.thread.IsOutdated {
		border = theme.LineNumber
	}
	// The whole card lights up when the cursor is anywhere on it.
	if view.rows[view.cursor].kind == rowThread && view.rows[view.cursor].thread == row.thread {
		border = theme.Focus
	}
	edge := func(text string) string { return paint(text, border, nil, false) }
	inner := clamp(limit-14, 30, 110)
	indent := "      "
	marker := " "
	if index == view.cursor {
		marker = paint("▌", theme.Focus, nil, false)
	}
	switch row.edge {
	case 'c':
		return fit(marker+indent+edge("▏ ")+row.text, limit)
	case 't':
		title := " " + row.text + " "
		return fit(marker+indent+edge("╭─")+title+edge(strings.Repeat("─", max(inner-width(title), 0))+"─╮"), limit)
	case 'b':
		action := " " + theme.faint().Render(row.text) + " "
		return fit(marker+indent+edge("╰─")+action+edge(strings.Repeat("─", max(inner-width(action), 0))+"─╯"), limit)
	}
	return fit(marker+indent+edge("│ ")+fit(row.text, inner)+edge(" │"), limit)
}

func (view *diffView) codeRow(app *App, row diffRow, index, limit int, rail func(color.Color) string) string {
	theme := app.theme
	file := view.files[row.file]
	styling := view.stylingFor(app, row.file)
	onCursor := index == view.cursor || view.selected(index)
	dim := view.hunkRead(app, file, file.Hunks[row.hunk])
	digits := view.digits()
	numberTone := theme.LineNumber
	if onCursor {
		numberTone = theme.PrimaryText
	}

	// look decides a line's sign, sign colour and the backgrounds behind it.
	look := func(line *diff.Line) (sign string, tone, background, emphasis color.Color) {
		sign, tone = " ", theme.LineNumber
		switch line.Kind {
		case diff.Added:
			sign, tone, background, emphasis = "+", theme.AddedSign, theme.AddedLine, theme.AddedWord
		case diff.Deleted:
			sign, tone, background, emphasis = "-", theme.RemovedSign, theme.RemovedLine, theme.RemovedWord
		}
		if dim {
			background, emphasis = nil, nil
		}
		switch {
		case !onCursor:
		case background == nil:
			background = theme.CursorLine
		default:
			background, emphasis = blend(background, theme.PrimaryText, 0.10), blend(emphasis, theme.PrimaryText, 0.10)
		}
		return
	}
	number := func(line *diff.Line, old bool) string {
		switch {
		case line == nil:
			return strings.Repeat(" ", digits)
		case old:
			return fmt.Sprintf("%*d", digits, line.OldNumber)
		}
		return fmt.Sprintf("%*d", digits, line.NewNumber)
	}

	if !view.rowsSplit {
		// One line per row: old number, new number, sign, code.
		line := row.right
		if line == nil {
			line = row.left
		}
		sign, tone, background, emphasis := look(line)
		gutter := paint(number(row.left, true)+" "+number(row.right, false)+" ", numberTone, background, false) + paint(sign+" ", tone, background, true)
		return rail(tone) + gutter + view.codeCell(theme, styling, line, background, emphasis, limit-2*digits-5, dim)
	}

	half := func(line *diff.Line, old bool, codeWidth int) (string, color.Color) {
		if line == nil {
			return strings.Repeat(" ", digits+3+codeWidth), theme.LineNumber
		}
		sign, tone, background, emphasis := look(line)
		gutter := paint(number(line, old)+" ", numberTone, background, false) + paint(sign+" ", tone, background, true)
		return gutter + view.codeCell(theme, styling, line, background, emphasis, codeWidth, dim), tone
	}
	leftWidth := limit / 2
	leftContent, leftTone := half(row.left, true, leftWidth-digits-4)
	rightContent, rightTone := half(row.right, false, limit-leftWidth-digits-4)
	return rail(leftTone) + leftContent + rail(rightTone) + rightContent
}

// digits is the width of the widest line number in the diff.
func (view *diffView) digits() int {
	largest := 0
	for _, file := range view.files {
		if count := len(file.Hunks); count > 0 {
			last := file.Hunks[count-1]
			largest = max(largest, last.OldStart+last.OldLines, last.NewStart+last.NewLines)
		}
	}
	return max(len(strconv.Itoa(largest)), 3)
}

// codeCell draws one line of code: syntax colours in front, the line's
// background behind, a stronger background on the words that changed, cut
// to the horizontal scroll window and padded to the cell.
func (view *diffView) codeCell(theme Theme, styling *fileStyling, line *diff.Line, background, emphasis color.Color, limit int, dim bool) string {
	if limit <= 0 {
		return ""
	}
	tokens := styling.tokens[line]
	if tokens == nil {
		tokens = []diff.Token{{Text: line.Text}}
	}
	spans := styling.spans[line]
	matches := view.matchSpans(line.Text)

	var builder strings.Builder
	column, written, offset := 0, 0, 0
	emit := func(text string, foreground color.Color, emphasised, matched, bold bool) {
		if strings.Contains(text, "\t") {
			var expanded strings.Builder
			position := column
			for _, character := range text {
				if character == '\t' {
					spaces := 4 - position%4
					expanded.WriteString(strings.Repeat(" ", spaces))
					position += spaces
				} else {
					expanded.WriteRune(character)
					position++
				}
			}
			text = expanded.String()
		}
		start := column
		column += width(text)
		from, to := max(start, view.offset), min(column, view.offset+limit)
		if from >= to {
			return
		}
		if from > start || to < column {
			text = ansi.Cut(text, from-start, to-start)
		}
		behind := background
		if emphasised && emphasis != nil {
			behind = emphasis
		}
		if dim {
			foreground, bold = theme.FaintText, false
		}
		if matched {
			behind, bold = theme.SearchMatch, true
		}
		builder.WriteString(paint(text, foreground, behind, bold))
		written += to - from
	}
	for _, token := range tokens {
		text := token.Text
		for text != "" {
			emphasised, boundary := false, math.MaxInt
			for _, span := range spans {
				if offset < span.Start {
					boundary = span.Start
					break
				}
				if offset < span.End {
					emphasised, boundary = true, span.End
					break
				}
			}
			matched := false
			for _, match := range matches {
				if offset < match.Start {
					boundary = min(boundary, match.Start)
					break
				}
				if offset < match.End {
					matched, boundary = true, min(boundary, match.End)
					break
				}
			}
			take := min(len(text), boundary-offset)
			emit(text[:take], tokenColor(token.Color), emphasised, matched, token.Bold)
			text, offset = text[take:], offset+take
		}
	}
	if written < limit {
		builder.WriteString(paint(strings.Repeat(" ", limit-written), nil, background, false))
	}
	return builder.String()
}

type treeNode struct {
	name     string
	file     int
	children []*treeNode
}

type treeRow struct {
	depth int
	label string
	// -1 for a directory.
	file int
}

// fileTree turns the flat file list into indented rows. A directory with a
// single subdirectory is joined with it ("a/b/c/"), which keeps deep
// monorepo paths from eating the whole pane.
func fileTree(files []diff.File) []treeRow {
	root := &treeNode{file: -1}
	for index, file := range files {
		node := root
		parts := strings.Split(file.Path(), "/")
		for depth, part := range parts {
			var child *treeNode
			for _, candidate := range node.children {
				if candidate.name == part && candidate.file < 0 && depth < len(parts)-1 {
					child = candidate
				}
			}
			if child == nil {
				child = &treeNode{name: part, file: -1}
				if depth == len(parts)-1 {
					child.file = index
				}
				node.children = append(node.children, child)
			}
			node = child
		}
	}
	var rows []treeRow
	var walk func(node *treeNode, depth int)
	walk = func(node *treeNode, depth int) {
		for _, child := range node.children {
			if child.file >= 0 {
				rows = append(rows, treeRow{depth: depth, label: child.name, file: child.file})
				continue
			}
			label := child.name
			for len(child.children) == 1 && child.children[0].file < 0 {
				child = child.children[0]
				label += "/" + child.name
			}
			rows = append(rows, treeRow{depth: depth, label: label + "/", file: -1})
			walk(child, depth+1)
		}
	}
	walk(root, 0)
	return rows
}

func (view *diffView) sidebar(app *App, limit, height int) []string {
	theme := app.theme
	viewedFiles, readHunks, totalHunks := view.progress(app)
	lines := []string{
		" " + theme.bold().Render("Files") + theme.faint().Render(fmt.Sprintf("  %d/%d viewed", viewedFiles, len(view.files))),
		" " + theme.faint().Render(fmt.Sprintf("hunks  %d/%d read", readHunks, totalHunks)),
		"",
	}
	currentFile := -1
	if row := view.currentRow(); row != nil {
		currentFile = row.file
	}
	tree := fileTree(view.files)
	selected := 0
	var body []string
	for index, row := range tree {
		indent := strings.Repeat("  ", row.depth)
		if row.file < 0 {
			body = append(body, " "+indent+theme.faint().Render(truncate(row.label, limit-2-len(indent))))
			continue
		}
		file := view.files[row.file]
		path := file.Path()
		viewed := view.viewed[path] == "VIEWED"
		status := map[diff.Status]color.Color{diff.AddedFile: theme.AddedSign, diff.DeletedFile: theme.RemovedSign, diff.Renamed: theme.Note, diff.Modified: theme.Note}[file.Status]

		right := ""
		if open := threadCount(view.threads[path]); open > 0 {
			right += theme.text(theme.Note).Render(fmt.Sprintf("●%d ", open))
		}
		switch {
		case viewed:
			right += theme.success().Render("✓")
		case view.viewed[path] == "DISMISSED":
			right += theme.warning().Render("↻")
		default:
			right += theme.success().Render(fmt.Sprintf("+%d", file.Additions)) + " " + theme.failure().Render(fmt.Sprintf("-%d", file.Deletions))
		}
		name := theme.primary().Render(row.label)
		if viewed {
			name = theme.faint().Render(row.label)
		}
		left := indent + paint(string(file.Status), status, nil, false) + " " + name
		line := spread(truncate(left, limit-width(right)-3), right, limit-2)
		if row.file == currentFile {
			selected = index
			line = paint("▌", theme.Focus, nil, false) + paintBackground(line+" ", theme.SelectedBackground)
		} else {
			line = " " + line
		}
		body = append(body, line)
	}
	// Keep the current file in view, roughly centred, in a long tree.
	room := height - len(lines)
	first := clamp(selected-room/2, 0, max(len(body)-room, 0))
	return append(lines, body[first:]...)
}
