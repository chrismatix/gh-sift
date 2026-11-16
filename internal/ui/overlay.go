package ui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type overlayKind int

const (
	overlayMenu overlayKind = iota
	overlayComposer
	overlayInput
	overlayConfirm
)

// Rows of text in the composer; with title, hint and border it takes 8 more.
const composerRows = 6

type menuItem struct {
	label string
	hint  string
	value string
}

// overlay is the one modal thing on screen: a menu, a multi-line composer,
// a one-line input, or a y/N question in the footer.
type overlay struct {
	kind  overlayKind
	title string
	// Menus filter as you type when filterable is set.
	items      []menuItem
	filterable bool
	filter     string
	cursor     int
	textarea   textarea.Model
	input      textinput.Model
	submit     func(app *App, value string) tea.Cmd
	// A review summary may be empty; every other composer needs text.
	allowEmpty bool
}

func (app *App) openMenu(title string, items []menuItem, submit func(app *App, value string) tea.Cmd) {
	app.overlay = &overlay{kind: overlayMenu, title: title, items: items, submit: submit}
}

func (app *App) openComposer(title, initial string, submit func(app *App, value string) tea.Cmd) {
	area := textarea.New()
	area.ShowLineNumbers = false
	area.Prompt = ""
	area.CharLimit = 0
	area.SetWidth(min(app.width-6, 100))
	area.SetHeight(composerRows)
	area.SetValue(initial)
	area.Focus()
	app.overlay = &overlay{kind: overlayComposer, title: title, textarea: area, submit: submit}
}

func (app *App) openInput(title, initial string, submit func(app *App, value string) tea.Cmd) {
	input := textinput.New()
	input.Prompt = ""
	input.SetWidth(min(app.width-10, 80))
	input.SetValue(initial)
	input.Focus()
	app.overlay = &overlay{kind: overlayInput, title: title, input: input, submit: submit}
}

func (app *App) openConfirm(question string, confirmed func(app *App) tea.Cmd) {
	app.overlay = &overlay{kind: overlayConfirm, title: question, submit: func(app *App, _ string) tea.Cmd { return confirmed(app) }}
}

func (current *overlay) visibleItems() []menuItem {
	if current.filter == "" {
		return current.items
	}
	var matching []menuItem
	needle := strings.ToLower(current.filter)
	for _, item := range current.items {
		if strings.Contains(strings.ToLower(item.label), needle) {
			matching = append(matching, item)
		}
	}
	return matching
}

func (app *App) updateOverlay(message tea.Msg) tea.Cmd {
	current := app.overlay
	key, isKey := message.(tea.KeyPressMsg)
	if isKey && (key.String() == "esc" || key.String() == "ctrl+c") {
		app.overlay = nil
		return nil
	}
	submit := func(value string) tea.Cmd {
		app.overlay = nil
		return current.submit(app, value)
	}

	switch current.kind {
	case overlayConfirm:
		if isKey {
			app.overlay = nil
			if key.String() == "y" || key.String() == "Y" {
				return current.submit(app, "")
			}
		}
	case overlayMenu:
		if !isKey {
			return nil
		}
		items := current.visibleItems()
		switch pressed := key.String(); {
		case pressed == "enter":
			if len(items) > 0 {
				return submit(items[current.cursor].value)
			}
		case pressed == "down" || pressed == "ctrl+n" || (pressed == "j" && !current.filterable):
			current.cursor = min(current.cursor+1, max(len(items)-1, 0))
		case pressed == "up" || pressed == "ctrl+p" || (pressed == "k" && !current.filterable):
			current.cursor = max(current.cursor-1, 0)
		case current.filterable && pressed == "backspace":
			if current.filter != "" {
				current.filter = current.filter[:len(current.filter)-1]
				current.cursor = 0
			}
		case current.filterable && key.Text != "":
			current.filter += key.Text
			current.cursor = 0
		}
	case overlayComposer:
		// ctrl+s is hunk's save, ctrl+d is gh-dash's submit.
		if isKey && (key.String() == "ctrl+s" || key.String() == "ctrl+d") {
			if value := strings.TrimSpace(current.textarea.Value()); value != "" || current.allowEmpty {
				return submit(value)
			}
			return nil
		}
		var command tea.Cmd
		current.textarea, command = current.textarea.Update(message)
		return command
	case overlayInput:
		if isKey && key.String() == "enter" {
			return submit(strings.TrimSpace(current.input.Value()))
		}
		var command tea.Cmd
		current.input, command = current.input.Update(message)
		return command
	}
	return nil
}

// overlayBox renders the overlay as a bordered box, or "" for the footer
// question, which the footer draws itself.
func (app *App) overlayBox() string {
	current := app.overlay
	theme := app.theme
	var body, hint string
	switch current.kind {
	case overlayConfirm:
		return ""
	case overlayMenu:
		var lines []string
		if current.filterable {
			lines = append(lines, theme.faint().Render("› ")+current.filter+theme.faint().Render("▏"), "")
		}
		items := current.visibleItems()
		// A long menu scrolls around the cursor.
		visible := clamp(app.height-10, 3, 20)
		first := clamp(current.cursor-visible/2, 0, max(len(items)-visible, 0))
		for index := first; index < min(first+visible, len(items)); index++ {
			item := items[index]
			line := "  " + item.label
			if index == current.cursor {
				line = theme.text(theme.Focus).Bold(true).Render("› " + item.label)
			}
			if item.hint != "" {
				line += "  " + theme.faint().Render(item.hint)
			}
			lines = append(lines, line)
		}
		if len(items) == 0 {
			lines = append(lines, theme.faint().Render("  nothing matches"))
		}
		body, hint = strings.Join(lines, "\n"), "enter select · esc cancel"
	case overlayComposer:
		body, hint = current.textarea.View(), "ctrl+s send · esc cancel"
	case overlayInput:
		body, hint = current.input.View(), "enter confirm · esc cancel"
	}
	content := theme.bold().Render(current.title) + "\n\n" + body + "\n\n" + theme.faint().Render(hint)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.Focus).
		Padding(0, 2).MaxWidth(app.width - 2).Render(content)
}

// overlayOnto draws a box over the screen lines: centred, or docked at the
// bottom so what it is about stays visible above it.
func overlayOnto(lines []string, box string, screenWidth int, docked bool) []string {
	boxLines := strings.Split(box, "\n")
	boxWidth := 0
	for _, line := range boxLines {
		boxWidth = max(boxWidth, width(line))
	}
	left := max((screenWidth-boxWidth)/2, 0)
	top := max((len(lines)-len(boxLines))/2, 0)
	if docked {
		top = max(len(lines)-len(boxLines), 0)
	}
	for offset, line := range boxLines {
		row := top + offset
		if row >= len(lines) {
			break
		}
		padded := line + strings.Repeat(" ", boxWidth-width(line))
		lines[row] = ansi.Cut(lines[row], 0, left) + padded + ansi.Cut(lines[row], left+boxWidth, screenWidth)
	}
	return lines
}
