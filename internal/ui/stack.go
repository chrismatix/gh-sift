package ui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chrismatix/gh-sift/internal/gh"
)

// topFirst orders a stack the way it is always drawn: the top of the stack
// first and the PR that sits on the base branch last, so moving up on screen
// is moving up the stack.
func topFirst(stack *gh.Stack) []gh.StackEntry {
	entries := slices.Clone(stack.Entries)
	slices.SortFunc(entries, func(first, second gh.StackEntry) int { return second.Position - first.Position })
	return entries
}

// stackLines draws the stack with the current PR marked, both ends labelled
// and the base branch underneath, because "up" and "down" are otherwise easy
// to get backwards.
func stackLines(theme Theme, stack *gh.Stack, current, limit int) []string {
	entries := topFirst(stack)
	if len(entries) == 0 {
		return nil
	}
	top, bottom := entries[0].Position, entries[len(entries)-1].Position
	// Merged PRs stay in the stack; what merges first is the lowest open one.
	for _, entry := range entries {
		if entry.State == "OPEN" {
			bottom = entry.Position
		}
	}
	positionWidth := len(strconv.Itoa(top))

	var lines []string
	for _, entry := range entries {
		note := ""
		switch entry.Position {
		case top:
			note = theme.stack().Render("▲ top")
		case bottom:
			note = theme.stack().Render("▼ merges first")
		}
		marker, title := "  ", theme.secondary().Render(fmt.Sprintf("#%d %s", entry.Number, entry.Title))
		if entry.State != "OPEN" {
			title = theme.faint().Render(fmt.Sprintf("#%d %s", entry.Number, entry.Title))
		}
		if entry.Position == current {
			marker = theme.stack().Bold(true).Render("▸ ")
			title = theme.bold().Render(fmt.Sprintf("#%d %s", entry.Number, entry.Title))
		}
		state := theme.stateGlyph(entry.State, entry.IsDraft, entry.IsInMergeQueue)
		if entry.ChecksState != "" && entry.State == "OPEN" {
			state += " " + theme.checkMark(entry.ChecksState)
		}
		left := marker + theme.stack().Render(fmt.Sprintf("%*d", positionWidth, entry.Position)) + " " + state + " "
		lines = append(lines, spread(left+truncate(title, limit-width(left)-width(note)-2), note, limit))
	}
	return append(lines, strings.Repeat(" ", positionWidth+2)+theme.faint().Render("└ "+stack.BaseRefName))
}

// stackBadge is the short form for headers and table cells: "3/5".
func stackBadge(position int, stack *gh.Stack) string {
	if stack == nil {
		return ""
	}
	return fmt.Sprintf("%d/%d", position, stack.Size)
}

// startMerge asks how to merge. A stacked PR can only merge together with
// everything below it, so the choices spell out which PRs go in.
func (app *App) startMerge(key pullRequestKey) tea.Cmd {
	detail := app.data(key).detail
	if detail == nil {
		app.notify("still loading this PR, try again in a moment", false)
		return app.loadPullRequest(key, false)
	}
	if detail.State != "OPEN" {
		app.notify(fmt.Sprintf("#%d is %s", detail.Number, map[string]string{"MERGED": "already merged", "CLOSED": "closed"}[detail.State]), false)
		return nil
	}

	labels := map[string]string{"--squash": "Squash and merge", "--merge": "Create a merge commit", "--rebase": "Rebase and merge"}
	methods := detail.MergeMethods
	if detail.MergeQueueEnabled {
		// The queue picks the method itself.
		methods, labels = []string{""}, map[string]string{"": "Add to the merge queue"}
	}

	var items []menuItem
	if detail.Stack == nil {
		// gh stack merge has no auto-merge, so this is for single PRs only.
		if !detail.MergeQueueEnabled && detail.ChecksState == "PENDING" {
			items = append(items, menuItem{label: "Auto-merge when checks pass", hint: "squash", value: "0|--auto"})
		}
		for _, method := range methods {
			items = append(items, menuItem{label: labels[method], value: "0|" + method})
		}
	} else {
		entries := topFirst(detail.Stack)
		topNumber := entries[0].Number
		upToHere := fmt.Sprintf("#%d and the %s below", detail.Number, plural(detail.StackPosition-1, "PR"))
		if detail.StackPosition == 1 {
			upToHere = fmt.Sprintf("only #%d, the bottom of the stack", detail.Number)
		}
		for _, method := range methods {
			items = append(items, menuItem{label: labels[method], hint: upToHere, value: fmt.Sprintf("%d|%s", detail.Number, method)})
		}
		if detail.StackPosition < detail.Stack.Size {
			for _, method := range methods {
				items = append(items, menuItem{label: labels[method], hint: fmt.Sprintf("the whole stack, all %d PRs up to #%d", detail.Stack.Size, topNumber), value: fmt.Sprintf("%d|%s", topNumber, method)})
			}
		}
	}
	if len(items) == 0 {
		app.notify("this repository allows no merge method", true)
		return nil
	}

	title := fmt.Sprintf("Merge #%d", detail.Number)
	if detail.Stack != nil {
		title = fmt.Sprintf("Merge stack · #%d is %d of %d", detail.Number, detail.StackPosition, detail.Stack.Size)
	}
	app.openMenu(title, items, func(app *App, value string) tea.Cmd {
		var target int
		var method string
		fmt.Sscanf(value, "%d|%s", &target, &method)
		stacked := target != 0
		if !stacked {
			target = detail.Number
		}
		done := fmt.Sprintf("merged #%d", target)
		switch {
		case method == "--auto":
			done = fmt.Sprintf("auto-merge enabled for #%d", target)
		case detail.MergeQueueEnabled:
			done = fmt.Sprintf("added #%d to the merge queue", target)
		case stacked:
			done = fmt.Sprintf("merged the stack up to #%d", target)
		}
		return app.run("merging", func() error { return app.client.Merge(key.repository, target, method, stacked) }, done, func(app *App) tea.Cmd {
			return tea.Batch(app.loadPullRequest(key, true), app.list.refreshCurrent(app))
		})
	})
	return nil
}
