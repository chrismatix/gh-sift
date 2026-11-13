package diff

import (
	"reflect"
	"testing"
)

const sample = `diff --git a/cmd/main.go b/cmd/main.go
index 1111111..2222222 100644
--- a/cmd/main.go
+++ b/cmd/main.go
@@ -10,6 +10,7 @@ func main() {
 	first()
-	old := compute(1)
-	print(old)
+	value := compute(2)
+	print(value)
+	log(value)
 	last()

 	done()
@@ -40,3 +41,3 @@ func other() {
 	a()
--- a/looks/like/header
+++ b/looks/like/header
 	b()
diff --git a/docs/old name.md b/docs/new name.md
similarity index 90%
rename from docs/old name.md
rename to docs/new name.md
index 3333333..4444444 100644
--- a/docs/old name.md
+++ b/docs/new name.md
@@ -1 +1 @@
-title
+Title
\ No newline at end of file
diff --git a/new.txt b/new.txt
new file mode 100644
index 0000000..5555555
--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+one
+two
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
index 6666666..0000000
--- a/gone.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
diff --git a/logo.png b/logo.png
index 7777777..8888888 100644
Binary files a/logo.png and b/logo.png differ
`

func TestParse(t *testing.T) {
	files := Parse(sample)
	if len(files) != 5 {
		t.Fatalf("got %d files", len(files))
	}

	main := files[0]
	if main.Path() != "cmd/main.go" || main.Status != Modified || main.Additions != 4 || main.Deletions != 3 {
		t.Errorf("main.go: %+v", main)
	}
	if len(main.Hunks) != 2 || main.Hunks[0].Section != "func main() {" {
		t.Fatalf("hunks: %+v", main.Hunks)
	}
	first := main.Hunks[0]
	if len(first.Lines) != 9 {
		t.Fatalf("first hunk has %d lines", len(first.Lines))
	}
	if got := first.Lines[1]; got.Kind != Deleted || got.OldNumber != 11 || got.NewNumber != 0 {
		t.Errorf("deleted line: %+v", got)
	}
	if got := first.Lines[5]; got.Kind != Added || got.NewNumber != 13 || got.Text != "\tlog(value)" {
		t.Errorf("added line: %+v", got)
	}
	if got := first.Lines[8]; got.Kind != Context || got.OldNumber != 15 || got.NewNumber != 16 {
		t.Errorf("last context line: %+v", got)
	}
	// Lines that look like file headers inside a hunk are content.
	second := main.Hunks[1]
	if second.Lines[1].Kind != Deleted || second.Lines[1].Text != "-- a/looks/like/header" || second.Lines[2].Kind != Added {
		t.Errorf("header lookalikes: %+v", second.Lines)
	}

	renamed := files[1]
	if renamed.Status != Renamed || renamed.OldPath != "docs/old name.md" || renamed.Path() != "docs/new name.md" {
		t.Errorf("rename: %+v", renamed)
	}
	if len(renamed.Hunks[0].Lines) != 2 {
		t.Errorf("no-newline marker should not be a line: %+v", renamed.Hunks[0].Lines)
	}
	if files[2].Status != AddedFile || files[2].Path() != "new.txt" || files[2].Additions != 2 {
		t.Errorf("added file: %+v", files[2])
	}
	if files[3].Status != DeletedFile || files[3].Path() != "gone.txt" {
		t.Errorf("deleted file: %+v", files[3])
	}
	if !files[4].Binary || files[4].Path() != "logo.png" {
		t.Errorf("binary file: %+v", files[4])
	}
}

func TestHunkHashIgnoresPositionAndContext(t *testing.T) {
	before := Parse(sample)[0].Hunks[0]
	moved := Parse(`diff --git a/cmd/main.go b/cmd/main.go
--- a/cmd/main.go
+++ b/cmd/main.go
@@ -110,5 +115,6 @@ func renamed() {
 	different context
-	old := compute(1)
-	print(old)
+	value := compute(2)
+	print(value)
+	log(value)
 	last()
`)[0].Hunks[0]
	if before.Hash() != moved.Hash() {
		t.Error("a moved hunk should keep its hash")
	}
	if before.Hash() == Parse(sample)[0].Hunks[1].Hash() {
		t.Error("different hunks should differ")
	}
}

func TestPairs(t *testing.T) {
	pairs := Parse(sample)[0].Hunks[0].Pairs()
	var shape []string
	for _, pair := range pairs {
		switch {
		case pair.Left != nil && pair.Right != nil && pair.Left.Kind == Context:
			shape = append(shape, "context")
		case pair.Left != nil && pair.Right != nil:
			shape = append(shape, "changed")
		case pair.Right != nil:
			shape = append(shape, "added")
		default:
			shape = append(shape, "deleted")
		}
	}
	want := []string{"context", "changed", "changed", "added", "context", "context", "context"}
	if !reflect.DeepEqual(shape, want) {
		t.Errorf("got %v, want %v", shape, want)
	}
}

func TestInlineSpans(t *testing.T) {
	oldText, newText := "\told := compute(1)", "\tvalue := compute(2)"
	oldSpans, newSpans := InlineSpans(oldText, newText)
	changed := func(text string, spans []Span) []string {
		var parts []string
		for _, span := range spans {
			parts = append(parts, text[span.Start:span.End])
		}
		return parts
	}
	if got := changed(oldText, oldSpans); !reflect.DeepEqual(got, []string{"old", "1"}) {
		t.Errorf("old spans: %q", got)
	}
	if got := changed(newText, newSpans); !reflect.DeepEqual(got, []string{"value", "2"}) {
		t.Errorf("new spans: %q", got)
	}

	if oldSpans, newSpans := InlineSpans("return nil", "panic(fmt.Sprintf(\"unreachable: %v\", state))"); oldSpans != nil || newSpans != nil {
		t.Errorf("unrelated lines should not be marked: %v %v", oldSpans, newSpans)
	}
}
