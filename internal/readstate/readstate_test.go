package readstate

import (
	"path/filepath"
	"testing"
)

func TestReadMarksSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "read.json")
	store := Open(path)
	if store.IsRead("owner/repo", 7, "main.go", "aaa") {
		t.Fatal("nothing should be read yet")
	}
	if err := store.Set("owner/repo", 7, "main.go", true, "aaa", "bbb"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("owner/repo", 7, "main.go", false, "bbb"); err != nil {
		t.Fatal(err)
	}

	reopened := Open(path)
	if !reopened.IsRead("owner/repo", 7, "main.go", "aaa") {
		t.Error("aaa should be read after reopening")
	}
	if reopened.IsRead("owner/repo", 7, "main.go", "bbb") {
		t.Error("bbb was unmarked")
	}
	if reopened.IsRead("owner/repo", 8, "main.go", "aaa") || reopened.IsRead("owner/repo", 7, "other.go", "aaa") {
		t.Error("marks must not leak across PRs or files")
	}
}
