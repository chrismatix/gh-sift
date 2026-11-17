package ui

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/chrismatix/gh-sift/internal/diff"
	"github.com/chrismatix/gh-sift/internal/gh"
)

type diffLayout int

const (
	layoutAuto diffLayout = iota
	layoutSplit
	layoutStack
)

type diffRowKind int

const (
	rowBlank diffRowKind = iota
	rowFile
	rowHunk
	rowCode
	// rowFold stands in for something hidden: a read hunk or a folded file.
	rowFold
	rowThread
)

type diffRow struct {
	kind diffRowKind
	file int
	// -1 on rows that belong to the file as a whole.
	hunk int
	// A code row shows left (old side) and right (new side). In the stacked
	// layout a row is one line: left for deleted, right for added, both for context.
	left   *diff.Line
	right  *diff.Line
	thread *gh.Thread
	// Thread rows: 't'op, 'm'iddle and 'b'ottom of a card, or 'c' when the
	// whole thread is collapsed to one line.
	edge byte
	text string
}

type fileStyling struct {
	tokens map[*diff.Line][]diff.Token
	spans  map[*diff.Line][]diff.Span
}

type diffView struct {
	layout      diffLayout
	hideSidebar bool
	// Show read hunks dimmed instead of folding them away.
	dimRead bool
	// Leaving the diff leaves the PR when the diff is what was opened.
	returnToList bool
	// A thread to put the cursor on once the diff has loaded.
	pendingThread string

	files  []diff.File
	viewed map[string]string
	// From .gitattributes; overrides the built-in guess in isGenerated.
	generated map[string]bool
	threads   map[string][]*gh.Thread

	rows      []diffRow
	stale     bool
	rowsWidth int
	rowsSplit bool
	cursor    int
	top       int
	offset    int
	// Row where a line selection started, or -1.
	anchor int

	// Explicit fold choices by path; without one, viewed and generated files fold.
	folded map[string]bool
	// Read hunks and resolved threads the user opened anyway.
	unfolded map[string]bool
	styling  map[int]*fileStyling

	// Full-text search: typing while the query is being entered, then n/N.
	query  string
	typing bool
}

// invalidate asks for the rows to be rebuilt. The old rows stay until then,
// because the rebuild needs them to put the cursor back where it was.
func (view *diffView) invalidate() { view.stale = true }

// setData takes the PR's current files, threads and viewed marks from the store.
func (view *diffView) setData(app *App) {
	data := app.data(app.pullRequest.key)
	if len(data.files) != len(view.files) || (len(data.files) > 0 && &data.files[0] != &view.files[0]) {
		view.files, view.styling = data.files, map[int]*fileStyling{}
		// Rows of another diff say nothing about where the cursor belongs.
		view.rows = nil
	}
	if view.folded == nil {
		view.folded, view.unfolded, view.anchor = map[string]bool{}, map[string]bool{}, -1
	}
	view.generated = data.generated
	view.viewed, view.threads = map[string]string{}, map[string][]*gh.Thread{}
	if detail := data.detail; detail != nil {
		for _, file := range detail.Files {
			view.viewed[file.Path] = file.ViewedState
		}
		for index := range detail.Threads {
			thread := &detail.Threads[index]
			view.threads[thread.Path] = append(view.threads[thread.Path], thread)
		}
		view.rememberViewed(app)
	}
	view.invalidate()
}

// rememberViewed records the hunks of files GitHub says are viewed. If the
// file changes later and GitHub drops the mark, these still tell old from new.
func (view *diffView) rememberViewed(app *App) {
	key := app.pullRequest.key
	for _, file := range view.files {
		if view.viewed[file.Path()] != "VIEWED" {
			continue
		}
		var missing []string
		for _, hunk := range file.Hunks {
			if hash := hunk.Hash(); !app.readState.IsRead(key.repository, key.number, file.Path(), hash) {
				missing = append(missing, hash)
			}
		}
		if len(missing) > 0 {
			_ = app.readState.Set(key.repository, key.number, file.Path(), true, missing...)
		}
	}
}

var generatedSuffixes = []string{
	"bun.lock", "bun.lockb", "go.sum", "flake.lock", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
	"Cargo.lock", "uv.lock", "poetry.lock", "Gemfile.lock", "composer.lock",
	".min.js", ".min.css", ".map", ".pb.go", "_pb2.py", ".g.dart", ".snap",
}

func isGenerated(path string) bool {
	for _, suffix := range generatedSuffixes {
		if strings.HasSuffix(path, suffix) {
			return true
		}
	}
	return strings.Contains(path, ".generated.")
}

func (view *diffView) isFolded(file diff.File) bool {
	if folded, chosen := view.folded[file.Path()]; chosen {
		return folded
	}
	if view.viewed[file.Path()] == "VIEWED" {
		return true
	}
	if generated, declared := view.generated[file.Path()]; declared {
		return generated
	}
	return isGenerated(file.Path())
}

func (view *diffView) hunkRead(app *App, file diff.File, hunk diff.Hunk) bool {
	key := app.pullRequest.key
	return view.viewed[file.Path()] == "VIEWED" || app.readState.IsRead(key.repository, key.number, file.Path(), hunk.Hash())
}

func (view *diffView) sidebarWidth(app *App) int {
	if view.hideSidebar || app.width < 100 || len(view.files) == 0 {
		return 0
	}
	return clamp(app.width/5, 28, 46)
}

func (view *diffView) paneWidth(app *App) int {
	if sidebar := view.sidebarWidth(app); sidebar > 0 {
		return app.width - sidebar - 1
	}
	return app.width
}

func (view *diffView) split(app *App) bool {
	switch view.layout {
	case layoutSplit:
		return true
	case layoutStack:
		return false
	}
	return view.paneWidth(app) >= 130
}

// ensureRows rebuilds the row list when its inputs changed, keeping the
// cursor on the same line, hunk or thread.
func (view *diffView) ensureRows(app *App) {
	paneWidth, split := view.paneWidth(app), view.split(app)
	if view.rows != nil && !view.stale && view.rowsWidth == paneWidth && view.rowsSplit == split {
		return
	}
	view.stale = false
	var anchor diffRow
	hadCursor := view.cursor < len(view.rows)
	if hadCursor {
		anchor = view.rows[view.cursor]
	}
	distance := view.cursor - view.top

	view.rowsWidth, view.rowsSplit = paneWidth, split
	view.rows = view.buildRows(app, paneWidth, split)
	view.anchor = -1

	if view.pendingThread != "" {
		for index, row := range view.rows {
			if row.kind == rowThread && row.thread.ID == view.pendingThread {
				view.cursor, view.top, view.pendingThread = index, max(index-4, 0), ""
				return
			}
		}
	}
	if !hadCursor {
		view.cursor = view.firstStop()
		return
	}
	view.cursor = view.locate(anchor)
	view.top = max(view.cursor-distance, 0)
}

// locate finds the row that best matches one from before a rebuild.
func (view *diffView) locate(wanted diffRow) int {
	best := -1
	for index, row := range view.rows {
		if row.file != wanted.file {
			continue
		}
		if best < 0 {
			best = index
		}
		switch {
		case wanted.kind == rowThread && row.kind == rowThread && row.thread.ID == wanted.thread.ID:
			return index
		case wanted.kind == rowCode && row.kind == rowCode && ((wanted.right != nil && row.right == wanted.right) || (wanted.right == nil && row.left == wanted.left)):
			return index
		case row.hunk == wanted.hunk && wanted.hunk >= 0 && (row.kind == rowHunk || row.kind == rowFold):
			best = index
		}
	}
	return clamp(best, 0, max(len(view.rows)-1, 0))
}

func (view *diffView) firstStop() int {
	for index, row := range view.rows {
		if row.kind == rowHunk {
			return index
		}
	}
	return min(1, max(len(view.rows)-1, 0))
}

func (view *diffView) buildRows(app *App, paneWidth int, split bool) []diffRow {
	var rows []diffRow
	for fileIndex := range view.files {
		file := &view.files[fileIndex]
		if fileIndex > 0 {
			rows = append(rows, diffRow{kind: rowBlank, file: fileIndex, hunk: -1})
		}
		rows = append(rows, diffRow{kind: rowFile, file: fileIndex, hunk: -1})

		placed := map[*gh.Thread]bool{}
		threadsAt := func(row diffRow) []*gh.Thread {
			var found []*gh.Thread
			for _, thread := range view.threads[file.Path()] {
				onLeft := thread.Side == "LEFT" && row.left != nil && row.left.OldNumber == thread.Line && thread.Line > 0
				// A context line carries both numbers; a RIGHT thread sits on the new one.
				onRight := thread.Side != "LEFT" && row.right != nil && row.right.NewNumber == thread.Line && thread.Line > 0
				if (onLeft || onRight) && !placed[thread] {
					placed[thread] = true
					found = append(found, thread)
				}
			}
			return found
		}

		switch {
		case file.Binary && len(file.Hunks) == 0:
			rows = append(rows, diffRow{kind: rowFold, file: fileIndex, hunk: -1, text: "Binary file, not shown"})
		case len(file.Hunks) == 0:
			rows = append(rows, diffRow{kind: rowFold, file: fileIndex, hunk: -1, text: "No content changes"})
		case view.isFolded(*file):
			reason := "generated file"
			if view.viewed[file.Path()] == "VIEWED" {
				reason = "viewed"
			}
			rows = append(rows, diffRow{kind: rowFold, file: fileIndex, hunk: -1, text: fmt.Sprintf("▸ %s · %s folded · z unfolds", reason, plural(len(file.Hunks), "hunk"))})
		default:
			for hunkIndex := range file.Hunks {
				hunk := &file.Hunks[hunkIndex]
				if view.hunkRead(app, *file, *hunk) && !view.dimRead && !view.unfolded[file.Path()+hunk.Hash()] {
					rows = append(rows, diffRow{kind: rowFold, file: fileIndex, hunk: hunkIndex})
					// A folded hunk keeps its threads next to it.
					for _, thread := range view.threads[file.Path()] {
						start, count := hunk.NewStart, hunk.NewLines
						if thread.Side == "LEFT" {
							start, count = hunk.OldStart, hunk.OldLines
						}
						if !placed[thread] && thread.Line >= start && thread.Line < start+count {
							placed[thread] = true
							rows = append(rows, view.threadRows(app, thread, fileIndex, hunkIndex, paneWidth)...)
						}
					}
					continue
				}
				rows = append(rows, diffRow{kind: rowHunk, file: fileIndex, hunk: hunkIndex})
				var codeRows []diffRow
				if split {
					for _, pair := range hunk.Pairs() {
						codeRows = append(codeRows, diffRow{kind: rowCode, file: fileIndex, hunk: hunkIndex, left: pair.Left, right: pair.Right})
					}
				} else {
					for lineIndex := range hunk.Lines {
						line := &hunk.Lines[lineIndex]
						row := diffRow{kind: rowCode, file: fileIndex, hunk: hunkIndex}
						if line.Kind != diff.Added {
							row.left = line
						}
						if line.Kind != diff.Deleted {
							row.right = line
						}
						codeRows = append(codeRows, row)
					}
				}
				for _, row := range codeRows {
					rows = append(rows, row)
					for _, thread := range threadsAt(row) {
						rows = append(rows, view.threadRows(app, thread, fileIndex, hunkIndex, paneWidth)...)
					}
				}
			}
		}
		// Threads GitHub can no longer place (outdated, or the file is
		// folded) still belong to the file, so they follow it.
		for _, thread := range view.threads[file.Path()] {
			if !placed[thread] {
				rows = append(rows, view.threadRows(app, thread, fileIndex, -1, paneWidth)...)
			}
		}
	}
	return rows
}

// threadRows renders a review thread as a card under its line. Resolved and
// outdated threads take a single line until opened.
func (view *diffView) threadRows(app *App, thread *gh.Thread, file, hunk, paneWidth int) []diffRow {
	theme := app.theme
	inner := clamp(paneWidth-14, 30, 110)
	base := diffRow{kind: rowThread, file: file, hunk: hunk, thread: thread}
	row := func(edge byte, text string) diffRow {
		next := base
		next.edge, next.text = edge, text
		return next
	}

	state := theme.warning().Render("○ open")
	if thread.IsResolved {
		state = theme.success().Render("✓ resolved")
	}
	if thread.IsOutdated {
		state += theme.faint().Render(fmt.Sprintf(" · outdated, was line %d", thread.OriginalLine))
	}
	collapsed := (thread.IsResolved || thread.IsOutdated) && !view.unfolded[thread.ID]
	if collapsed {
		about := ""
		if len(thread.Comments) > 0 {
			about = theme.faint().Render(" · @"+thread.Comments[0].Author+": ") + theme.secondary().Render(summary(thread.Comments[0].Body))
		}
		return []diffRow{row('c', truncate(state+about, inner))}
	}

	title := state
	if len(thread.Comments) > 0 {
		title += severityTag(theme, severity(thread.Comments[0].Body))
	}
	rows := []diffRow{row('t', title)}
	for index, line := range threadComments(app, *thread, inner) {
		// threadComments opens with a blank line, which the top border replaces.
		if index > 0 {
			rows = append(rows, row('m', line))
		}
	}
	action := "r reply · x resolve"
	if thread.IsResolved {
		action = "r reply · x reopen · enter collapse"
	}
	return append(rows, row('b', action))
}

func (view *diffView) currentRow() *diffRow {
	if view.cursor < len(view.rows) {
		return &view.rows[view.cursor]
	}
	return nil
}

// moveTo places the cursor and scrolls just enough to keep it in view.
func (view *diffView) moveTo(app *App, index int) {
	view.cursor = clamp(index, 0, max(len(view.rows)-1, 0))
	height := view.bodyHeight(app)
	margin := min(3, height/4)
	view.top = clamp(view.top, view.cursor-height+1+margin, max(view.cursor-margin, 0))
	view.top = clamp(view.top, 0, max(len(view.rows)-height, 0))
}

// reveal puts a jump target a quarter down the pane, so what follows it
// (the hunk, the file) is on screen.
func (view *diffView) reveal(app *App, index int) {
	view.cursor = clamp(index, 0, max(len(view.rows)-1, 0))
	height := view.bodyHeight(app)
	view.top = clamp(view.cursor-max(2, height/4), 0, max(len(view.rows)-height, 0))
}

func (view *diffView) bodyHeight(app *App) int {
	// Screen minus the tab row, its rule and the footer.
	return max(app.height-3, 1)
}

func (view *diffView) step(app *App, delta int) {
	target := view.cursor + delta
	// Blank rows are spacing, not something to stand on.
	for target > 0 && target < len(view.rows)-1 && view.rows[target].kind == rowBlank {
		target += delta / max(abs(delta), 1)
	}
	view.moveTo(app, target)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// jump moves to the next row in a direction that satisfies the test.
func (view *diffView) jump(app *App, direction int, what string, test func(index int, row diffRow) bool) {
	for index := view.cursor + direction; index >= 0 && index < len(view.rows); index += direction {
		if test(index, view.rows[index]) {
			view.reveal(app, index)
			return
		}
	}
	app.notify(map[int]string{1: "no further ", -1: "no earlier "}[direction]+what, false)
}

// matchSpans finds the query in a line, ignoring case.
func (view *diffView) matchSpans(text string) []diff.Span {
	if view.query == "" {
		return nil
	}
	lower, query := strings.ToLower(text), strings.ToLower(view.query)
	// Lowercasing can change byte lengths outside ASCII, which would misplace the spans.
	if len(lower) != len(text) {
		return nil
	}
	var spans []diff.Span
	for start := 0; ; {
		found := strings.Index(lower[start:], query)
		if found < 0 {
			return spans
		}
		spans = append(spans, diff.Span{Start: start + found, End: start + found + len(query)})
		start += found + len(query)
	}
}

// findMatch moves to the next line containing the query, searching every
// file, and unfolds the file or read hunk the match sits in.
func (view *diffView) findMatch(app *App, direction int) {
	type place struct {
		file, hunk int
		line       *diff.Line
	}
	var places []place
	order := map[*diff.Line]int{}
	for fileIndex, file := range view.files {
		for hunkIndex := range file.Hunks {
			for lineIndex := range file.Hunks[hunkIndex].Lines {
				line := &file.Hunks[hunkIndex].Lines[lineIndex]
				order[line] = len(places)
				places = append(places, place{fileIndex, hunkIndex, line})
			}
		}
	}
	// Positions are doubled so a row between two lines can sit between their positions.
	low, high := 2*len(places), 2*len(places)
	for index := view.cursor; index < len(view.rows); index++ {
		row := view.rows[index]
		if row.kind != rowCode {
			continue
		}
		low, high = 1<<30, -1
		for _, line := range []*diff.Line{row.left, row.right} {
			if line != nil {
				low, high = min(low, 2*order[line]), max(high, 2*order[line])
			}
		}
		if index > view.cursor {
			low, high = low-1, low-1
		}
		break
	}
	target := -1
	for index := range places {
		if len(view.matchSpans(places[index].line.Text)) == 0 {
			continue
		}
		if direction > 0 && 2*index > high {
			target = index
			break
		}
		if direction < 0 && 2*index < low {
			target = index
		}
	}
	if target < 0 {
		app.notify(map[int]string{1: "no further ", -1: "no earlier "}[direction]+"match for "+view.query, false)
		return
	}
	found := places[target]
	file := view.files[found.file]
	if view.isFolded(file) {
		view.folded[file.Path()] = false
	}
	view.unfolded[file.Path()+file.Hunks[found.hunk].Hash()] = true
	view.invalidate()
	view.ensureRows(app)
	for index, row := range view.rows {
		if row.left == found.line || row.right == found.line {
			view.reveal(app, index)
			return
		}
	}
}

func (view *diffView) handleKey(app *App, pressed string) tea.Cmd {
	view.ensureRows(app)
	if view.typing {
		switch {
		case pressed == "esc":
			view.typing, view.query = false, ""
		case pressed == "enter":
			view.typing = false
			if view.query != "" {
				view.findMatch(app, 1)
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
		return nil
	}
	row := view.currentRow()
	half := max(view.bodyHeight(app)/2, 1)

	switch pressed {
	case "esc":
		if view.anchor >= 0 {
			view.anchor = -1
			return nil
		}
		if view.query != "" {
			view.query = ""
			return nil
		}
		fallthrough
	case "q":
		if view.returnToList {
			return app.leave(pressed)
		}
		app.pullRequest.tab = tabOverview
		return nil
	case "0":
		view.layout = layoutAuto
	case "1":
		view.layout = layoutSplit
	case "2":
		view.layout = layoutStack
	case "s":
		view.hideSidebar = !view.hideSidebar
	case "H":
		view.dimRead = !view.dimRead
		view.invalidate()
		app.notify(map[bool]string{true: "read hunks shown dimmed", false: "read hunks folded"}[view.dimRead], false)
	case "j", "down":
		view.step(app, 1)
	case "k", "up":
		view.step(app, -1)
	case "d", "ctrl+d":
		view.top += half
		view.moveTo(app, view.cursor+half)
	case "u", "ctrl+u":
		view.top -= half
		view.moveTo(app, view.cursor-half)
	case "space", "f", "pgdown":
		view.top += 2 * half
		view.moveTo(app, view.cursor+2*half)
	case "b", "pgup":
		view.top -= 2 * half
		view.moveTo(app, view.cursor-2*half)
	case "g", "home":
		view.moveTo(app, 0)
	case "G", "end":
		view.moveTo(app, len(view.rows)-1)
	case "left":
		view.offset = max(view.offset-8, 0)
	case "right":
		view.offset += 8
	case "]":
		view.jump(app, 1, "hunk", func(_ int, row diffRow) bool { return row.kind == rowHunk })
	case "[":
		view.jump(app, -1, "hunk", func(_ int, row diffRow) bool { return row.kind == rowHunk })
	case ".":
		view.jump(app, 1, "file", func(_ int, row diffRow) bool { return row.kind == rowFile })
	case ",":
		view.jump(app, -1, "file", func(_ int, row diffRow) bool { return row.kind == rowFile })
	case "}":
		view.jump(app, 1, "thread", func(_ int, row diffRow) bool {
			return row.kind == rowThread && (row.edge == 't' || row.edge == 'c') && row.thread != view.rows[view.cursor].thread
		})
	case "{":
		view.jump(app, -1, "thread", func(index int, row diffRow) bool {
			return row.kind == rowThread && (row.edge == 't' || row.edge == 'c') && row.thread != view.rows[view.cursor].thread
		})
	case "/":
		return view.pickFile(app)
	case "ctrl+f":
		view.typing, view.query = true, ""
	case "n", "N":
		if view.query == "" {
			app.notify("ctrl+f searches the diff", false)
			return nil
		}
		view.findMatch(app, map[string]int{"n": 1, "N": -1}[pressed])
	}
	if row == nil {
		return nil
	}
	file := view.files[row.file]

	switch pressed {
	case "V":
		if view.anchor >= 0 {
			view.anchor = -1
		} else if row.kind == rowCode {
			view.anchor = view.cursor
		}
	case "z":
		view.folded[file.Path()] = !view.isFolded(file)
		view.invalidate()
	case "enter":
		switch {
		case row.kind == rowThread:
			view.unfolded[row.thread.ID] = !view.unfolded[row.thread.ID]
		case row.kind == rowFold && row.hunk >= 0:
			view.unfolded[file.Path()+file.Hunks[row.hunk].Hash()] = true
		case row.kind == rowFold || row.kind == rowFile:
			view.folded[file.Path()] = !view.isFolded(file)
		}
		view.invalidate()
	case "v":
		return view.toggleViewed(app, file)
	case "x":
		if row.kind == rowThread {
			return app.toggleResolved(*row.thread)
		}
		if row.hunk >= 0 {
			return view.toggleRead(app, row.file, row.hunk)
		}
		app.notify("x checks off a hunk or a thread; v marks the whole file viewed", false)
	case "r":
		if row.kind == rowThread {
			return app.startReply(*row.thread)
		}
		app.notify("move onto a thread to reply (} jumps to the next one)", false)
	case "c", "S":
		return view.startComment(app, pressed == "S")
	}
	return nil
}

func (view *diffView) pickFile(app *App) tea.Cmd {
	var items []menuItem
	for index, file := range view.files {
		items = append(items, menuItem{label: file.Path(), hint: fmt.Sprintf("+%d -%d", file.Additions, file.Deletions), value: fmt.Sprint(index)})
	}
	app.openMenu("Jump to file", items, func(app *App, value string) tea.Cmd {
		var wanted int
		fmt.Sscan(value, &wanted)
		view := &app.pullRequest.diff
		for index, row := range view.rows {
			if row.kind == rowFile && row.file == wanted {
				view.reveal(app, index)
			}
		}
		return nil
	})
	app.overlay.filterable = true
	return nil
}

// setViewed updates GitHub's Viewed checkbox, optimistically on screen.
func (view *diffView) setViewed(app *App, path string, viewed bool) tea.Cmd {
	key := app.pullRequest.key
	detail := app.data(key).detail
	state := map[bool]string{true: "VIEWED", false: "UNVIEWED"}[viewed]
	for index := range detail.Files {
		if detail.Files[index].Path == path {
			detail.Files[index].ViewedState = state
		}
	}
	view.viewed[path] = state
	delete(view.folded, path)
	view.invalidate()
	return app.run("syncing viewed", func() error { return app.client.SetFileViewed(detail.ID, path, viewed) }, "", nil)
}

func (view *diffView) toggleViewed(app *App, file diff.File) tea.Cmd {
	key := app.pullRequest.key
	viewed := view.viewed[file.Path()] != "VIEWED"
	var hashes []string
	for _, hunk := range file.Hunks {
		hashes = append(hashes, hunk.Hash())
	}
	if err := app.readState.Set(key.repository, key.number, file.Path(), viewed, hashes...); err != nil {
		app.notify("could not save read state: "+firstLine(err), true)
	}
	return view.setViewed(app, file.Path(), viewed)
}

// toggleRead checks a hunk off locally. The file's Viewed mark on GitHub
// follows: set once every hunk is read, cleared when one is reopened.
func (view *diffView) toggleRead(app *App, fileIndex, hunkIndex int) tea.Cmd {
	key := app.pullRequest.key
	file := view.files[fileIndex]
	hash := file.Hunks[hunkIndex].Hash()
	read := !view.hunkRead(app, file, file.Hunks[hunkIndex])
	wasViewed := view.viewed[file.Path()] == "VIEWED"

	if err := app.readState.Set(key.repository, key.number, file.Path(), read, hash); err != nil {
		app.notify("could not save read state: "+firstLine(err), true)
	}
	delete(view.unfolded, file.Path()+hash)
	view.invalidate()

	allRead := true
	for _, hunk := range file.Hunks {
		allRead = allRead && app.readState.IsRead(key.repository, key.number, file.Path(), hunk.Hash())
	}
	var command tea.Cmd
	if allRead != wasViewed {
		command = view.setViewed(app, file.Path(), allRead)
	}
	if read {
		// Carry on to the next thing that still needs reading.
		view.ensureRows(app)
		view.jump(app, 1, "unread hunk", func(_ int, row diffRow) bool { return row.kind == rowHunk })
	}
	return command
}

// commentTarget works out which lines a new comment is about: the selection
// if there is one, else the cursor line. GitHub wants one side for the range.
func (view *diffView) commentTarget() (gh.NewComment, []string, bool) {
	first, last := view.cursor, view.cursor
	if view.anchor >= 0 {
		first, last = min(view.anchor, view.cursor), max(view.anchor, view.cursor)
	}
	end := view.rows[last]
	if end.kind != rowCode {
		return gh.NewComment{}, nil, false
	}
	comment := gh.NewComment{Path: view.files[end.file].Path(), Side: "RIGHT"}
	number := func(row diffRow) (int, string) {
		if comment.Side == "LEFT" {
			if row.left != nil {
				return row.left.OldNumber, row.left.Text
			}
			return 0, ""
		}
		if row.right != nil {
			return row.right.NewNumber, row.right.Text
		}
		return 0, ""
	}
	// A deleted line only exists on the old side.
	if end.right == nil {
		comment.Side = "LEFT"
	}
	var text []string
	for index := first; index <= last; index++ {
		row := view.rows[index]
		if row.kind != rowCode || row.file != end.file {
			continue
		}
		if line, content := number(row); line > 0 {
			if comment.StartLine == 0 {
				comment.StartLine = line
			}
			comment.Line = line
			text = append(text, content)
		}
	}
	return comment, text, comment.Line > 0
}

func (view *diffView) startComment(app *App, suggestion bool) tea.Cmd {
	comment, text, ok := view.commentTarget()
	if !ok {
		app.notify("move onto a line of code to comment on it", false)
		return nil
	}
	if suggestion && comment.Side == "LEFT" {
		app.notify("a suggestion replaces new code; this line was deleted", false)
		return nil
	}
	location := fmt.Sprintf("%s:%d", shortPath(comment.Path, 50), comment.Line)
	if comment.StartLine != comment.Line {
		location = fmt.Sprintf("%s:%d-%d", shortPath(comment.Path, 50), comment.StartLine, comment.Line)
	}
	title, initial := "Comment on "+location, ""
	if suggestion {
		title, initial = "Suggest a change to "+location, "```suggestion\n"+strings.Join(text, "\n")+"\n```"
	}
	key := app.pullRequest.key
	detail := app.data(key).detail
	// The composer docks at the bottom; keep the commented line above it.
	view.top = max(view.top, view.cursor-(view.bodyHeight(app)-composerRows-8))
	usesTabs := strings.Contains(strings.Join(text, "\n"), "\t")
	app.openComposer(title, initial, func(app *App, body string) tea.Cmd {
		// The text area turns tabs into four spaces, which would make a
		// suggestion re-indent tab-indented code.
		if suggestion && usesTabs {
			lines := strings.Split(body, "\n")
			for index, line := range lines {
				trimmed := strings.TrimLeft(line, " ")
				indent := len(line) - len(trimmed)
				lines[index] = strings.Repeat("\t", indent/4) + strings.Repeat(" ", indent%4) + trimmed
			}
			body = strings.Join(lines, "\n")
		}
		comment.Body = body
		app.pullRequest.diff.anchor = -1
		return app.run("adding comment", func() error { return app.client.AddReviewComment(detail.ID, comment) },
			"added to your pending review · a submits it", func(app *App) tea.Cmd { return app.loadPullRequest(key, true) })
	})
	return nil
}

func (view *diffView) hints(app *App) string {
	switch {
	case view.typing:
		return "enter find · esc cancel"
	case view.query != "":
		return "n/N next match · esc clears the search · j/k line · c comment"
	}
	if view.anchor >= 0 {
		return "selecting lines · j/k extend · c comment · S suggest · esc cancel"
	}
	if row := view.currentRow(); row != nil && row.kind == rowThread {
		return "r reply · x resolve · enter fold · } { thread · a review · esc back"
	}
	return "j/k line · ] [ hunk · . , file · x read · v viewed · c comment · a review · K/J stack · esc back"
}

// progress counts read hunks and viewed files for the sidebar heading.
func (view *diffView) progress(app *App) (viewedFiles, readHunks, totalHunks int) {
	for _, file := range view.files {
		if view.viewed[file.Path()] == "VIEWED" {
			viewedFiles++
		}
		for _, hunk := range file.Hunks {
			totalHunks++
			if view.hunkRead(app, file, hunk) {
				readHunks++
			}
		}
	}
	return
}

// stylingFor computes a file's syntax tokens and word-level marks on first use.
func (view *diffView) stylingFor(app *App, fileIndex int) *fileStyling {
	if styling := view.styling[fileIndex]; styling != nil {
		return styling
	}
	file := &view.files[fileIndex]
	styling := &fileStyling{tokens: map[*diff.Line][]diff.Token{}, spans: map[*diff.Line][]diff.Span{}}
	view.styling[fileIndex] = styling
	// Colouring a huge diff costs more than it helps anyone read it.
	if file.Additions+file.Deletions > 8000 {
		return styling
	}
	for hunkIndex := range file.Hunks {
		hunk := &file.Hunks[hunkIndex]
		var oldLines, newLines []*diff.Line
		for lineIndex := range hunk.Lines {
			line := &hunk.Lines[lineIndex]
			if line.Kind != diff.Added {
				oldLines = append(oldLines, line)
			}
			if line.Kind != diff.Deleted {
				newLines = append(newLines, line)
			}
		}
		for _, side := range [][]*diff.Line{oldLines, newLines} {
			texts := make([]string, len(side))
			for index, line := range side {
				texts[index] = line.Text
			}
			for index, tokens := range diff.Highlight(file.Path(), texts, app.theme.SyntaxStyleName) {
				styling.tokens[side[index]] = tokens
			}
		}
		for _, pair := range hunk.Pairs() {
			if pair.Left != nil && pair.Right != nil && pair.Left.Kind == diff.Deleted {
				styling.spans[pair.Left], styling.spans[pair.Right] = diff.InlineSpans(pair.Left.Text, pair.Right.Text)
			}
		}
	}
	return styling
}

// selected reports whether a row is inside the line selection.
func (view *diffView) selected(index int) bool {
	return view.anchor >= 0 && index >= min(view.anchor, view.cursor) && index <= max(view.anchor, view.cursor)
}

func threadCount(threads []*gh.Thread) (open int) {
	return len(slices.DeleteFunc(slices.Clone(threads), func(thread *gh.Thread) bool { return thread.IsResolved }))
}
