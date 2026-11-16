package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
	}
	for _, each := range cases {
		if !pullRequestReference.MatchString(each.target) {
			t.Errorf("%q should be read as a PR reference", each.target)
			continue
		}
		repository, number, tab, err := parseTarget(each.target, "here/repo")
		if err != nil || repository != each.repository || number != each.number || tab != each.tab {
			t.Errorf("%q = %s#%d tab %q (%v), want %s#%d tab %q", each.target, repository, number, tab, err, each.repository, each.number, each.tab)
		}
	}
	if _, _, _, err := parseTarget("123", ""); err == nil {
		t.Error("a bare number outside a repository cannot name a PR")
	}
	// Everything else is a branch name, slashes included.
	for _, branch := range []string{"HEAD", "main", "feat/stack-view", "release-1.2", "github.com/owner/repo/pull/45"} {
		if pullRequestReference.MatchString(branch) {
			t.Errorf("%q should be left to be read as a branch", branch)
		}
	}
}

// runEnvironment isolates run from the real repository, config and credentials.
func runEnvironment(t *testing.T, repository string) {
	t.Chdir(t.TempDir())
	t.Setenv("GH_REPO", repository)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GH_DASH_CONFIG", "")
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "test")
	t.Setenv("GH_ENTERPRISE_TOKEN", "test")
	t.Setenv("GH_HOST", "")
}

func TestRunRefusesTargetsItCannotResolve(t *testing.T) {
	cases := []struct {
		name       string
		repository string
		target     string
		message    string
	}{
		{"number outside a repository", "", "12", "not inside a repository: name the PR as owner/repo#12"},
		{"branch outside a repository", "", "feature", `"feature" names a branch, which needs a checkout of a GitHub repository on a branch`},
		{"HEAD off any branch", "o/r", "HEAD", `"HEAD" names a branch`},
	}
	for _, each := range cases {
		t.Run(each.name, func(t *testing.T) {
			runEnvironment(t, each.repository)
			if err := run("", each.target); err == nil || !strings.HasPrefix(err.Error(), each.message) {
				t.Errorf("run(%q) = %v, want %q", each.target, err, each.message)
			}
		})
	}
}

func TestRunReportsConfigAndCredentialErrors(t *testing.T) {
	runEnvironment(t, "")
	if err := run(filepath.Join(t.TempDir(), "missing.yml"), ""); err == nil || !strings.Contains(err.Error(), "reading config") {
		t.Errorf("a missing --config file should stop the start, got %v", err)
	}

	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	// go-gh falls back to `gh auth token`, which would find the keyring's token.
	t.Setenv("PATH", t.TempDir())
	if err := run("", ""); err == nil || !strings.Contains(err.Error(), "authentication token not found") {
		t.Errorf("no credentials should stop the start, got %v", err)
	}
}

func TestRunLooksUpTheBranchsPullRequest(t *testing.T) {
	runEnvironment(t, "127.0.0.1:1/o/r")
	// Nothing listens on port 1, so the lookup fails without leaving the machine.
	t.Setenv("GH_HOST", "127.0.0.1:1")
	if output, err := exec.Command("git", "init", "--quiet", "--initial-branch", "feature").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, output)
	}
	if err := run("", "HEAD"); err == nil || !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("the branch lookup's error should surface, got %v", err)
	}
}
