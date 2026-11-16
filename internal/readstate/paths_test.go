package readstate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/state")
	if got := DefaultPath(); got != "/state/gh-sift/read.json" {
		t.Errorf("DefaultPath() = %s", got)
	}
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/someone")
	if got := DefaultPath(); got != "/home/someone/.local/state/gh-sift/read.json" {
		t.Errorf("DefaultPath() = %s", got)
	}
}

func TestBrokenFileOpensEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "read.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := Open(path)
	if store.IsRead("o/r", 1, "a.go", "x") {
		t.Error("a broken file holds no marks")
	}
	if err := store.Set("o/r", 1, "a.go", true, "x", "x"); err != nil {
		t.Fatal(err)
	}
	if !Open(path).IsRead("o/r", 1, "a.go", "x") {
		t.Error("saving should replace the broken file")
	}
}

func TestSaveErrors(t *testing.T) {
	directory := t.TempDir()
	blocker := filepath.Join(directory, "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// A regular file where the directory should be.
	if err := Open(filepath.Join(blocker, "read.json")).Set("o/r", 1, "a.go", true, "x"); err == nil {
		t.Error("an uncreatable directory should be an error")
	}
	// A directory where the temporary file should be.
	path := filepath.Join(directory, "read.json")
	if err := os.Mkdir(path+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Open(path).Set("o/r", 1, "a.go", true, "x"); err == nil {
		t.Error("an unwritable file should be an error")
	}
}
