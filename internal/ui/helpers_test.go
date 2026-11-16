package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/chrismatix/gh-sift/internal/config"
	"github.com/chrismatix/gh-sift/internal/gh"
)

func TestHelperTextLayout(t *testing.T) {
	cases := []struct {
		name, got, want string
	}{
		{"truncate to nothing", truncate("abc", 0), ""},
		{"truncate", truncate("abcdef", 4), "abc…"},
		{"fit pads", fit("ab", 4), "ab  "},
		{"fit cuts", fit("abcdef", 4), "abc…"},
		{"fitRight pads on the left", fitRight("ab", 4), "  ab"},
		{"fitRight cuts", fitRight("abcdef", 4), "abc…"},
		{"spread", spread("left", "right", 12), "left   right"},
		{"spread squeezes the left side", spread("a long left", "right", 10), "a l… right"},
		{"spread without room", spread("left", "right", 3), " right"},
	}
	for _, each := range cases {
		if each.got != each.want {
			t.Errorf("%s = %q, want %q", each.name, each.got, each.want)
		}
	}

	if got := ansi.Strip(fill(lipgloss.NewStyle().Background(lipgloss.Color("1")), "x", 3)); got != "x  " {
		t.Errorf("fill = %q", got)
	}
	if got := beside([]string{"a", "b"}, "|", []string{"1", "2"}); !reflect.DeepEqual(got, []string{"a|1", "b|2"}) {
		t.Errorf("beside = %q", got)
	}
	if got := wrap("one two three", 7); !reflect.DeepEqual(got, []string{"one two", "three"}) {
		t.Errorf("wrap = %q", got)
	}
	if got := wrap("one two", 0); !reflect.DeepEqual(got, []string{"one two"}) {
		t.Errorf("wrap without a width leaves the text whole, got %q", got)
	}
	if got := clamp(9, 0, 5); got != 5 {
		t.Errorf("clamp = %d", got)
	}
}

func TestHelperAges(t *testing.T) {
	now := time.Now()
	cases := []struct {
		age  time.Duration
		want string
	}{
		{10 * time.Second, "now"},
		{45 * time.Second, "45s"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{2 * 24 * time.Hour, "2d"},
		{15 * 24 * time.Hour, "2w"},
		{60 * 24 * time.Hour, "2mo"},
		{800 * 24 * time.Hour, "2y"},
	}
	for _, each := range cases {
		if got := elapsed(now.Add(-each.age)); got != each.want {
			t.Errorf("elapsed(%v) = %q, want %q", each.age, got, each.want)
		}
	}
	if elapsed(time.Time{}) != "" || ago(time.Time{}) != "" {
		t.Error("an unknown time has no age")
	}
	if got := ago(now); got != "now" {
		t.Errorf("ago(now) = %q", got)
	}
	if got := ago(now.Add(-3 * time.Hour)); got != "3h ago" {
		t.Errorf("ago = %q", got)
	}
}

func TestHelperDurationAndCounts(t *testing.T) {
	start := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name, got, want string
	}{
		{"not started", duration(time.Time{}, start), ""},
		{"seconds", duration(start, start.Add(42*time.Second)), "42s"},
		{"minutes", duration(start, start.Add(125*time.Second)), "2m 05s"},
		{"clock skew", duration(start, start.Add(-time.Second)), "0s"},
		{"small number", shortNumber(999), "999"},
		{"large number", shortNumber(1250), "1.2k"},
		{"one", plural(1, "file"), "1 file"},
		{"many", plural(3, "file"), "3 files"},
		{"none", plural(0, "file"), "0 files"},
	}
	for _, each := range cases {
		if each.got != each.want {
			t.Errorf("%s = %q, want %q", each.name, each.got, each.want)
		}
	}
	if running := duration(time.Now().Add(-90*time.Second), time.Time{}); running != "1m 30s" {
		t.Errorf("a running check counts up to now, got %q", running)
	}
}

func TestHelperThemeOverrides(t *testing.T) {
	light, dark := NewTheme(false, nil), NewTheme(true, nil)
	if light.Dark || !dark.Dark || light.SyntaxStyleName != "github" || dark.SyntaxStyleName != "github-dark" {
		t.Errorf("light %v %s, dark %v %s", light.Dark, light.SyntaxStyleName, dark.Dark, dark.SyntaxStyleName)
	}
	if light.PrimaryText == dark.PrimaryText {
		t.Error("light and dark should differ")
	}

	colors := &config.ColorTheme{
		Text: config.ColorThemeText{
			Primary: "#000001", Secondary: "#000002", Faint: "#000003", Inverted: "#000004",
			Success: "#000005", Warning: "#000006", Error: "#000007",
		},
		Border:     config.ColorThemeBorder{Primary: "#000008", Secondary: "#000009", Faint: "#00000A"},
		Background: config.ColorThemeBackground{Selected: "#00000B"},
	}
	for _, dark := range []bool{false, true} {
		theme := NewTheme(dark, colors)
		got := []any{
			theme.PrimaryText, theme.SecondaryText, theme.FaintText, theme.InvertedText,
			theme.SuccessText, theme.WarningText, theme.ErrorText,
			theme.PrimaryBorder, theme.SecondaryBorder, theme.FaintBorder, theme.SelectedBackground,
		}
		for index, value := range []string{"#000001", "#000002", "#000003", "#000004", "#000005", "#000006", "#000007", "#000008", "#000009", "#00000A", "#00000B"} {
			if got[index] != lipgloss.Color(value) {
				t.Errorf("dark=%v: colour %d = %v, want %s", dark, index, got[index], value)
			}
		}
	}
	// An empty override keeps the default.
	if theme := NewTheme(true, &config.ColorTheme{}); theme.PrimaryText != dark.PrimaryText {
		t.Error("an unset colour should keep the default")
	}
}

func TestHelperThemeGlyphs(t *testing.T) {
	theme := NewTheme(true, nil)
	cases := []struct {
		name, got, want string
	}{
		{"merged", theme.stateGlyph("MERGED", false, false), glyphMergedPR},
		{"closed", theme.stateGlyph("CLOSED", true, false), glyphClosedPR},
		{"queued", theme.stateGlyph("OPEN", false, true), glyphMergeQueue},
		{"draft", theme.stateGlyph("OPEN", true, false), glyphDraftPR},
		{"open", theme.stateGlyph("OPEN", false, false), glyphOpenPR},
		{"merged label", theme.stateLabel("MERGED", false, false), " Merged "},
		{"closed label", theme.stateLabel("CLOSED", false, false), " Closed "},
		{"queued label", theme.stateLabel("OPEN", true, true), " Queued "},
		{"draft label", theme.stateLabel("OPEN", true, false), " Draft "},
		{"open label", theme.stateLabel("OPEN", false, false), " Open "},
		{"checks passed", theme.checksGlyph("SUCCESS"), glyphSuccess},
		{"checks pending", theme.checksGlyph("EXPECTED"), glyphWaiting},
		{"checks failed", theme.checksGlyph("ERROR"), glyphFailure},
		{"no checks", theme.checksGlyph(""), glyphNone},
		{"check passed", theme.checkMark("SUCCESS"), "✓"},
		{"check pending", theme.checkMark("PENDING"), "●"},
		{"check failed", theme.checkMark("FAILURE"), "✗"},
		{"check unknown", theme.checkMark("NEUTRAL"), "·"},
		{"approved", theme.reviewGlyph("APPROVED", 0), glyphApproved},
		{"changes requested", theme.reviewGlyph("CHANGES_REQUESTED", 1), glyphChanges},
		{"commented", theme.reviewGlyph("", 2), glyphCommented},
		{"waiting", theme.reviewGlyph("REVIEW_REQUIRED", 0), glyphWaiting},
	}
	for _, each := range cases {
		if got := ansi.Strip(each.got); got != each.want {
			t.Errorf("%s = %q, want %q", each.name, got, each.want)
		}
	}
	// Failure and warning must be told apart by colour, not only by glyph.
	if theme.failure().Render("x") == theme.warning().Render("x") {
		t.Error("failure and warning render the same")
	}
}

func TestHelperSummaryAndSeverity(t *testing.T) {
	cases := []struct {
		body, summary, severity string
	}{
		{"## Heading only\n\n```suggestion", "Heading only", ""},
		{"# Title\n\n- **Use** [the docs](https://x.y) and `fmt`", "Use the docs and fmt", ""},
		{"**Major** · Correctness\n\nReal text", "Real text", "major"},
		{"\n\n**Minor** · nit\n\nRename this.", "Rename this.", "minor"},
		{"Plain first line\n**Critical** · later", "Plain first line", ""},
		{"<!-- only a comment -->", "", ""},
	}
	for _, each := range cases {
		if got := summary(each.body); got != each.summary {
			t.Errorf("summary(%q) = %q, want %q", each.body, got, each.summary)
		}
		if got := severity(each.body); got != each.severity {
			t.Errorf("severity(%q) = %q, want %q", each.body, got, each.severity)
		}
	}
	if got := cleanBody("<img width=\"40\" alt=\"shot\" src=\"x.png\">"); got != "[image: shot]" {
		t.Errorf("an image with alt text should keep it: %q", got)
	}
	if got := cleanBody("<img src=\"y.png\"> and ![](z.png)\r\n\n\n\nmore"); got != "and \n\nmore" {
		t.Errorf("cleanBody = %q", got)
	}
}

func TestHelperMarkdown(t *testing.T) {
	for _, dark := range []bool{false, true} {
		app := &App{theme: NewTheme(dark, nil)}
		if lines := app.markdown("<!-- hidden -->", 40); lines != nil {
			t.Errorf("an empty body renders nothing, got %q", lines)
		}
		lines := app.markdown("## Heading\n\nSome `code` and a long sentence that has to wrap somewhere.\n\n```suggestion\nfixed()\n```", 30)
		var plain []string
		for _, line := range lines {
			plain = append(plain, ansi.Strip(line))
			if width(line) > 30 {
				t.Errorf("line wider than the limit: %q", ansi.Strip(line))
			}
			if strings.HasSuffix(line, " ") {
				t.Errorf("trailing spaces are trimmed: %q", line)
			}
		}
		text := strings.Join(plain, "\n")
		for _, expected := range []string{"Heading", "Some code and", "Suggested change", "fixed()"} {
			if !strings.Contains(text, expected) {
				t.Errorf("dark=%v: rendered markdown lacks %q:\n%s", dark, expected, text)
			}
		}
		if strings.Contains(text, "## Heading") {
			t.Error("headings are drawn without their hashes")
		}
	}
}

func TestHelperKeymap(t *testing.T) {
	keys, err := newKeymap(config.Keybindings{
		Universal: []config.Keybinding{{Key: "Q", Builtin: "quit", Name: "leave"}},
		Prs: []config.Keybinding{
			{Key: "M", Builtin: "merge"},
			{Key: "d", Command: "echo {{.PrNumber}}"},
			{Key: "z", Name: "nothing to run"},
		},
		Issues: []config.Keybinding{{Key: "E", Command: "a very long command that does\nnot fit in the help"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if match(keys.universal, "Q") != "quit" || match(keys.universal, "q") != "" || match(keys.prs, "M") != "merge" {
		t.Error("a builtin rebinding replaces its keys")
	}
	if len(keys.customPRs) != 1 || keys.customPRs[0].Key != "d" {
		t.Errorf("only entries with a command are custom: %+v", keys.customPRs)
	}

	help := helpFor(keys.prs, keys.customPRs...)
	if !reflect.DeepEqual(help[:2], []helpEntry{{"enter", "open the PR"}, {"K", "up the stack"}}) {
		t.Errorf("a key taken by a custom command leaves the help, and keyless builtins never show: %+v", help[:2])
	}
	if quit := helpFor(keys.universal); quit[len(quit)-1] != (helpEntry{"Q", "leave"}) {
		t.Errorf("a rebinding's name becomes its help: %+v", quit[len(quit)-1])
	}
	custom := helpForCustom(append(keys.customPRs, config.Keybinding{Key: "w", Name: "worktree"}, keys.customIssues[0]))
	want := []helpEntry{{"d", "echo {{.PrNumber}}"}, {"w", "worktree"}, {"E", "a very long command that does…"}}
	if !reflect.DeepEqual(custom, want) {
		t.Errorf("custom help = %+v", custom)
	}

	for _, configured := range []config.Keybindings{
		{Universal: []config.Keybinding{{Key: "x", Builtin: "nope"}}},
		{Prs: []config.Keybinding{{Key: "x", Builtin: "nope"}}},
		{Issues: []config.Keybinding{{Key: "x", Builtin: "nope"}}},
	} {
		if _, err := newKeymap(configured); err == nil || !strings.Contains(err.Error(), "unknown built-in") {
			t.Errorf("an unknown builtin should be refused, got %v", err)
		}
	}
}

func TestHelperSectionCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	if len(loadSectionCache()) != 0 {
		t.Error("no cache yet")
	}
	saved := map[string]cachedSection{"prs/Mine": {TotalCount: 1, PullRequests: []gh.PullRequestRow{{Number: 7, Title: "Add sift"}}}}
	saveSectionCache(saved)
	if loaded := loadSectionCache(); !reflect.DeepEqual(loaded, saved) {
		t.Errorf("loaded = %+v", loaded)
	}

	// A file where the cache directory should be: saving gives up quietly.
	blocked := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", blocked)
	t.Setenv("HOME", blocked)
	directory := filepath.Dir(sectionCachePath())
	if err := os.MkdirAll(filepath.Dir(directory), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(directory, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	saveSectionCache(saved)
	if len(loadSectionCache()) != 0 {
		t.Error("nothing could have been saved")
	}

	// Without a home there is no cache directory at all.
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	if sectionCachePath() != "" || len(loadSectionCache()) != 0 {
		t.Error("without a cache directory nothing is cached")
	}
	saveSectionCache(saved)
}
