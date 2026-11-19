package ui

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/chrismatix/gh-sift/internal/gh"
)

// cachedSection is the last result of one section's search. It lets the
// list paint at once on start while the real search, which takes a few
// seconds, runs behind it.
type cachedSection struct {
	TotalCount    int
	PullRequests  []gh.PullRequestRow
	Issues        []gh.IssueRow
	Notifications []gh.Notification
}

func sectionCachePath() string {
	directory, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(directory, "gh-sift", "sections.json")
}

func loadSectionCache() map[string]cachedSection {
	cache := map[string]cachedSection{}
	if contents, err := os.ReadFile(sectionCachePath()); err == nil {
		_ = json.Unmarshal(contents, &cache)
	}
	return cache
}

// saveSectionCache is best effort: a cache that cannot be written only
// costs the instant first paint.
func saveSectionCache(cache map[string]cachedSection) {
	path := sectionCachePath()
	contents, err := json.Marshal(cache)
	if path == "" || err != nil || os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	_ = os.WriteFile(path, contents, 0o644)
}
