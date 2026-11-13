// Package readstate remembers which hunks of which PR have been read.
package readstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// Store maps "owner/repo#number" to file path to the hashes of read hunks.
type Store struct {
	path string
	read map[string]map[string][]string
}

func DefaultPath() string {
	directory := os.Getenv("XDG_STATE_HOME")
	if directory == "" {
		home, _ := os.UserHomeDir()
		directory = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(directory, "gh-sift", "read.json")
}

// Open never fails on a missing or broken file: read marks are a
// convenience, and losing them must not keep the app from starting.
func Open(path string) *Store {
	store := &Store{path: path, read: map[string]map[string][]string{}}
	if contents, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(contents, &store.read)
	}
	return store
}

func key(repository string, number int) string {
	return fmt.Sprintf("%s#%d", repository, number)
}

func (store *Store) IsRead(repository string, number int, path, hash string) bool {
	return slices.Contains(store.read[key(repository, number)][path], hash)
}

// Set marks hunks of one file read or unread and saves.
func (store *Store) Set(repository string, number int, path string, read bool, hashes ...string) error {
	files := store.read[key(repository, number)]
	if files == nil {
		files = map[string][]string{}
		store.read[key(repository, number)] = files
	}
	for _, hash := range hashes {
		index := slices.Index(files[path], hash)
		switch {
		case read && index < 0:
			files[path] = append(files[path], hash)
		case !read && index >= 0:
			files[path] = slices.Delete(files[path], index, index+1)
		}
	}
	if len(files[path]) == 0 {
		delete(files, path)
	}
	return store.save()
}

func (store *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(store.path), 0o755); err != nil {
		return err
	}
	contents, err := json.MarshalIndent(store.read, "", " ")
	if err != nil {
		return err
	}
	// Write then rename, so a crash cannot leave a half-written file.
	temporary := store.path + ".tmp"
	if err := os.WriteFile(temporary, contents, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, store.path)
}
