package diff

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHighlight(t *testing.T) {
	lines := []string{"/* a comment", "   spanning lines */", "func main() {}"}
	highlighted := Highlight("main.go", lines, "monokai")
	if len(highlighted) != len(lines) {
		t.Fatalf("one token list per line, got %d", len(highlighted))
	}
	for index, tokens := range highlighted {
		var text strings.Builder
		for _, token := range tokens {
			text.WriteString(token.Text)
		}
		if text.String() != lines[index] {
			t.Errorf("line %d reads %q, want %q", index, text.String(), lines[index])
		}
	}
	// The second line only knows it is a comment from the first.
	if comment := highlighted[1][0]; comment.Color == "" || comment.Color != highlighted[0][0].Color {
		t.Errorf("a block comment should keep its colour across lines: %+v vs %+v", highlighted[1], highlighted[0])
	}
	keywordBold := false
	for _, token := range Highlight("main.go", []string{"package main"}, "github")[0] {
		if token.Text == "package" {
			keywordBold = token.Bold
		}
	}
	if !keywordBold {
		t.Error("the github style sets keywords bold")
	}
}

func TestHighlightUnknownLanguageIsPlain(t *testing.T) {
	lines := []string{"just text", ""}
	want := [][]Token{{{Text: "just text"}}, {{Text: ""}}}
	if got := Highlight("notes.unknown-extension", lines, "monokai"); !reflect.DeepEqual(got, want) {
		t.Errorf("Highlight = %+v", got)
	}
}

func TestGeneratedByAttributesShortCircuits(t *testing.T) {
	for _, each := range []struct {
		attributes string
		paths      []string
	}{
		{"*.go text eol=lf\n", []string{"main.go"}},
		{"*.pb.go linguist-generated\n", nil},
	} {
		declared, err := GeneratedByAttributes(each.attributes, each.paths)
		if err != nil || len(declared) != 0 {
			t.Errorf("GeneratedByAttributes(%q, %v) = %v, %v", each.attributes, each.paths, declared, err)
		}
	}
}

func TestGeneratedByAttributesWhenGitFails(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := GeneratedByAttributes("*.pb.go linguist-generated\n", []string{"a.pb.go"}); err == nil {
		t.Error("without git the attributes cannot be evaluated")
	}

	// A git whose init works but whose check-attr fails.
	directory := t.TempDir()
	script := "#!/bin/sh\n[ \"$1\" = init ]\n"
	if err := os.WriteFile(filepath.Join(directory, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	if _, err := GeneratedByAttributes("*.pb.go linguist-generated\n", []string{"a.pb.go"}); err == nil {
		t.Error("a failing check-attr should be an error")
	}
}

func TestInlineSpansSkipsVeryLongLines(t *testing.T) {
	long := strings.Repeat("word ", 600)
	if oldSpans, newSpans := InlineSpans(long, long+"x"); oldSpans != nil || newSpans != nil {
		t.Error("lines too long for the LCS table get no inline marks")
	}
}

func TestPathsFromGitHeader(t *testing.T) {
	cases := []struct {
		header   string
		old, new string
	}{
		{`diff --git "a/with space.go" "b/with space.go"`, "with space.go", "with space.go"},
		{`diff --git "a/tab\there.go" "b/tab\there.go"`, "tab\there.go", "tab\there.go"},
		{"diff --git a/old.go b/new.go", "old.go", "new.go"},
		{"diff --git a/a b/c b/a b/c", "a b/c", "a b/c"},
		{"diff --git weird", "weird", "weird"},
		{`diff --git "a/unterminated`, `"a/unterminated`, `"a/unterminated`},
	}
	for _, each := range cases {
		old, new := pathsFromGitHeader(each.header)
		if old != each.old || new != each.new {
			t.Errorf("%q = %q, %q; want %q, %q", each.header, old, new, each.old, each.new)
		}
	}
	if got := unquote(`"bad \q escape"`); got != `"bad \q escape"` {
		t.Errorf("an invalid quoted path is kept as is, got %q", got)
	}
}
