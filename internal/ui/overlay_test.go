package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"
)

// listOpenMenu opens a menu of the given labels whose choice lands in chosen.
func listOpenMenu(app *testApp, labels []string, filterable bool, chosen *string) {
	var items []menuItem
	for _, label := range labels {
		items = append(items, menuItem{label: label, hint: "hint for " + label, value: strings.ToLower(label)})
	}
	app.openMenu("Pick a file", items, func(_ *App, value string) tea.Cmd {
		*chosen = value
		return nil
	})
	app.overlay.filterable = filterable
}

func TestOverlayMenuMovesAndSelects(t *testing.T) {
	app := newTestApp(t, Options{Config: listTestConfig()})
	var chosen string
	listOpenMenu(app, []string{"Alpha", "Beta", "Gamma"}, false, &chosen)
	app.expectScreen("Pick a file", "› Alpha", "hint for Beta", "enter select · esc cancel")
	steps := []struct {
		key    string
		cursor int
	}{
		{"j", 1}, {"down", 2}, {"ctrl+n", 2}, {"k", 1}, {"up", 0}, {"ctrl+p", 0}, {"ctrl+n", 1},
	}
	for _, step := range steps {
		app.press(step.key)
		if app.overlay.cursor != step.cursor {
			t.Fatalf("after %s the cursor is %d, want %d", step.key, app.overlay.cursor, step.cursor)
		}
	}
	app.send(tea.PasteMsg{Content: "ignored"})
	app.press("enter")
	if chosen != "beta" || app.overlay != nil {
		t.Errorf("chose %q, overlay %v", chosen, app.overlay)
	}

	listOpenMenu(app, []string{"Alpha"}, false, &chosen)
	app.press("esc")
	if app.overlay != nil || chosen != "beta" {
		t.Error("esc should close the menu without choosing")
	}
}

func TestOverlayMenuFilters(t *testing.T) {
	app := newTestApp(t, Options{Config: listTestConfig()})
	var chosen string
	listOpenMenu(app, []string{"cmd/main.go", "internal/ui/app.go", "internal/ui/list.go"}, true, &chosen)
	app.press("j")
	if app.overlay.filter != "j" || app.overlay.cursor != 0 {
		t.Fatal("in a filterable menu j is typed, not a move")
	}
	app.press("backspace", "backspace", "UI/", "down")
	app.expectScreen("› UI/▏", "internal/ui/list.go")
	if strings.Contains(app.screen(), "cmd/main.go") {
		t.Error("the filter should hide what does not match")
	}
	app.press("zzz")
	app.expectScreen("nothing matches")
	app.press("enter")
	if app.overlay == nil {
		t.Fatal("enter on an empty menu should do nothing")
	}
	app.press("backspace", "backspace", "backspace", "down", "enter")
	if chosen != "internal/ui/list.go" {
		t.Errorf("chose %q", chosen)
	}
}

func TestOverlayLongMenuScrollsWithTheCursor(t *testing.T) {
	app := newTestApp(t, Options{Config: listTestConfig()})
	var labels []string
	for index := range 40 {
		labels = append(labels, fmt.Sprintf("item %02d", index))
	}
	var chosen string
	listOpenMenu(app, labels, false, &chosen)
	app.press("G")
	for range 30 {
		app.press("j")
	}
	app.expectScreen("› item 30")
	if strings.Contains(app.screen(), "item 01 ") {
		t.Error("the top of a long menu should scroll away")
	}
}

func TestOverlayComposerAndInput(t *testing.T) {
	app := newTestApp(t, Options{Config: listTestConfig()})
	var sent []string
	record := func(_ *App, value string) tea.Cmd { sent = append(sent, value); return nil }

	app.openComposer("Review summary", "", record)
	app.overlay.allowEmpty = true
	app.press("ctrl+s")
	app.openComposer("Reply", "  draft  ", record)
	app.expectScreen("Reply", "draft")
	app.press("ctrl+d")
	app.openInput("Labels", "bug", record)
	app.expectScreen("enter confirm · esc cancel")
	app.send(tea.PasteMsg{Content: ",ux "})
	app.press("enter")
	if want := []string{"", "draft", "bug,ux"}; strings.Join(sent, "|") != strings.Join(want, "|") {
		t.Errorf("sent %q, want %q", sent, want)
	}

	app.openInput("Labels", "", record)
	app.press("ctrl+c")
	if app.overlay != nil || len(sent) != 3 {
		t.Error("ctrl+c should cancel an input")
	}
}

func TestOverlayComposerIsDockedAtTheBottom(t *testing.T) {
	app := newTestApp(t, Options{Config: listTestConfig()})
	app.openComposer("Comment on #101", "", func(*App, string) tea.Cmd { return nil })
	lines := strings.Split(app.screen(), "\n")
	if !strings.Contains(lines[len(lines)-3], "ctrl+s send") {
		t.Errorf("the composer should sit just above the footer:\n%s", app.screen())
	}
}

func TestOverlayOnto(t *testing.T) {
	screen := []string{"..........", "..........", "..........", ".........."}
	tests := []struct {
		name   string
		box    string
		docked bool
		want   []string
	}{
		{"centred", "ab\ncd", false, []string{"..........", "....ab....", "....cd....", ".........."}},
		{"docked", "ab\ncd", true, []string{"..........", "..........", "....ab....", "....cd...."}},
		{"ragged lines are padded", "abc\nd", false, []string{"..........", "...abc....", "...d  ....", ".........."}},
		{"taller than the screen", "a\nb\nc\nd\ne\nf", false, []string{"....a.....", "....b.....", "....c.....", "....d....."}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := overlayOnto(append([]string(nil), screen...), test.box, 10, test.docked)
			for index := range got {
				got[index] = ansi.Strip(got[index])
			}
			if strings.Join(got, "\n") != strings.Join(test.want, "\n") {
				t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(test.want, "\n"))
			}
		})
	}
}
