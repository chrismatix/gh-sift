package ui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chrismatix/gh-sift/internal/config"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

// testApp is an App wired to a fake GitHub, with its state and cache in a
// scratch directory and a browser that does nothing.
type testApp struct {
	*App
	t      *testing.T
	github *ghtest.Server
}

func newTestApp(t *testing.T, options Options) *testApp {
	t.Helper()
	scratch := t.TempDir()
	t.Setenv("XDG_STATE_HOME", scratch+"/state")
	t.Setenv("XDG_CACHE_HOME", scratch+"/cache")
	t.Setenv("HOME", scratch)
	t.Setenv("GH_BROWSER", "true")
	t.Setenv("SIFT_READ_ONLY", "")
	server, client := ghtest.New(t)
	server.GraphQL("viewer", ghtest.Reply(200, map[string]any{"viewer": map[string]any{"login": "octocat"}}))
	server.GraphQL("search(", ghtest.Reply(200, map[string]any{"search": map[string]any{"nodes": []any{}, "issueCount": 0}}))
	options.Client = client
	if options.Config.PRSections == nil && options.Config.IssuesSections == nil {
		options.Config = config.Default()
	}
	app, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	harness := &testApp{App: app, t: t, github: server}
	harness.send(tea.WindowSizeMsg{Width: 140, Height: 40})
	harness.run(app.Init())
	return harness
}

// send delivers a message and runs every command it leads to.
func (harness *testApp) send(message tea.Msg) {
	harness.t.Helper()
	_, command := harness.Update(message)
	harness.run(command)
}

// run executes a command and feeds its messages back in. A command that
// does not answer quickly is a timer (tea.Tick) and is dropped: tests send
// tickMsg themselves when they want one.
func (harness *testApp) run(command tea.Cmd) {
	harness.t.Helper()
	pending := []tea.Cmd{command}
	for steps := 0; len(pending) > 0; steps++ {
		if steps > 500 {
			harness.t.Fatal("commands never settled")
		}
		next := pending[0]
		pending = pending[1:]
		if next == nil {
			continue
		}
		result := make(chan tea.Msg, 1)
		go func() { result <- next() }()
		var message tea.Msg
		select {
		case message = <-result:
		case <-time.After(100 * time.Millisecond):
			continue
		}
		switch message := message.(type) {
		case nil, tea.QuitMsg:
		case tea.BatchMsg:
			pending = append(pending, message...)
		default:
			_, command := harness.Update(message)
			pending = append(pending, command)
		}
	}
}

// press types keys: each argument is a key name ("enter", "ctrl+s", "esc")
// or a run of plain characters ("jjk"), each sent as its own key.
func (harness *testApp) press(keys ...string) {
	harness.t.Helper()
	for _, name := range keys {
		if key, named := namedKey(name); named {
			harness.send(key)
			continue
		}
		for _, character := range name {
			harness.send(tea.KeyPressMsg{Code: character, Text: string(character)})
		}
	}
}

func namedKey(name string) (tea.KeyPressMsg, bool) {
	codes := map[string]rune{
		"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "space": tea.KeySpace,
		"backspace": tea.KeyBackspace, "up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft,
		"right": tea.KeyRight, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd,
	}
	modifier := tea.KeyMod(0)
	base := name
	if rest, found := strings.CutPrefix(name, "ctrl+"); found {
		modifier, base = tea.ModCtrl, rest
	} else if rest, found := strings.CutPrefix(name, "shift+"); found {
		modifier, base = tea.ModShift, rest
	}
	if code, known := codes[base]; known {
		return tea.KeyPressMsg{Code: code, Mod: modifier}, true
	}
	if modifier != 0 && utf8.RuneCountInString(base) == 1 {
		character, _ := utf8.DecodeRuneInString(base)
		return tea.KeyPressMsg{Code: character, Mod: modifier}, true
	}
	return tea.KeyPressMsg{}, false
}

// screen is the rendered view without colours.
func (harness *testApp) screen() string {
	return ansi.Strip(harness.View().Content)
}

func (harness *testApp) expectScreen(texts ...string) {
	harness.t.Helper()
	screen := harness.screen()
	for _, text := range texts {
		if !strings.Contains(screen, text) {
			harness.t.Fatalf("screen lacks %q:\n%s", text, screen)
		}
	}
}
