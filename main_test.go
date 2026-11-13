package main

import "testing"

func TestParseTarget(t *testing.T) {
	cases := []struct {
		target     string
		repository string
		number     int
		tab        string
	}{
		{"123", "here/repo", 123, ""},
		{"owner/repo#45", "owner/repo", 45, ""},
		{"https://github.com/owner/repo/pull/45", "owner/repo", 45, ""},
		{"https://github.com/owner/my.repo/pull/45/files", "owner/my.repo", 45, "files"},
		{"https://github.com/owner/repo/pull/45/changes#diff-0a1b", "owner/repo", 45, "changes"},
		{"https://github.com/owner/repo/pull/45/checks?check_run_id=9", "owner/repo", 45, "checks"},
		{"https://github.com/owner/repo/pull/45#issuecomment-7", "owner/repo", 45, ""},
		{"github.com/owner/repo/pull/45", "", 0, ""},
	}
	for _, each := range cases {
		repository, number, tab, err := parseTarget(each.target, "here/repo")
		if each.number == 0 {
			if err == nil {
				t.Errorf("%q should not parse, got %s#%d", each.target, repository, number)
			}
			continue
		}
		if err != nil || repository != each.repository || number != each.number || tab != each.tab {
			t.Errorf("%q = %s#%d tab %q (%v), want %s#%d tab %q", each.target, repository, number, tab, err, each.repository, each.number, each.tab)
		}
	}
	if _, _, _, err := parseTarget("123", ""); err == nil {
		t.Error("a bare number outside a repository cannot name a PR")
	}
}
