package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, directory, name, contents string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A trimmed copy of a real gh-dash config: every construct it uses must parse.
const realWorldConfig = `
prSections:
    - title: My Pull Requests
      filters: is:open author:@me
    - title: Recently closed
      filters: is:closed author:@me closed:>2026-06-01 sort:updated-desc
      limit: 5
issuesSections:
    - title: My Issues
      filters: is:open author:@me
defaults:
    preview:
        open: true
        width: 0.45
    prsLimit: 30
    prApproveComment: LGTM
    view: prs
    layout:
        prs:
            repo:
                width: 20
            labels:
                width: 22
                hidden: true
    refetchIntervalMinutes: 30
keybindings:
    prs:
        - key: W
          name: agent
          command: >-
            $HOME/.config/gh-dash/pr-actions.sh worktree
            {{.RepoPath}} {{.RepoName}} {{.PrNumber}} {{.HeadRefName}}
        - key: M
          builtin: merge
repoPaths:
    "acme/core": ~/coding/acme/core
    ":owner/:repo": ~/coding/:repo
theme:
    ui:
        sectionsShowCount: true
        table:
            showSeparator: true
            compact: false
    colors:
        text:
            primary: "#E2E1ED"
pager:
    diff: gh-dash-hunk-pager
confirmQuit: false
showAuthorIcons: true
smartFilteringAtLaunch: true
`

func TestLoadGhDashConfig(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)
	t.Setenv("GH_DASH_CONFIG", "")
	path := writeConfig(t, directory, "config.yml", realWorldConfig)

	config, err := Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(config.PRSections) != 2 || config.PRSections[1].Title != "Recently closed" || *config.PRSections[1].Limit != 5 {
		t.Errorf("sections not replaced by the file: %+v", config.PRSections)
	}
	if config.Defaults.PrsLimit != 30 {
		t.Errorf("prsLimit = %d", config.Defaults.PrsLimit)
	}
	if config.Defaults.IssuesLimit != 20 || config.Defaults.Preview.Height != 0.60 {
		t.Errorf("unset values lost their defaults: %+v", config.Defaults)
	}
	if !*config.Defaults.Layout.Prs.Labels.Hidden || *config.Defaults.Layout.Prs.Lines.Width != 15 {
		t.Errorf("layout merge wrong: %+v", config.Defaults.Layout.Prs)
	}
	if len(config.Keybindings.Prs) != 2 || config.Keybindings.Prs[0].Name != "agent" || config.Keybindings.Prs[1].Builtin != "merge" {
		t.Errorf("keybindings: %+v", config.Keybindings.Prs)
	}
	if config.Theme.Colors.Text.Primary != "#E2E1ED" || !config.Theme.Ui.Table.ShowSeparator {
		t.Errorf("theme: %+v", config.Theme)
	}
	if config.Pager.Diff != "gh-dash-hunk-pager" {
		t.Errorf("pager: %+v", config.Pager)
	}
}

func TestLayersUnionKeybindingsAndReplaceSections(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", directory)
	t.Setenv("GH_DASH_CONFIG", "")
	if err := os.MkdirAll(filepath.Join(directory, "gh-dash"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, filepath.Join(directory, "gh-dash"), "config.yml", `
prSections:
    - title: Global
      filters: is:open
keybindings:
    prs:
        - key: W
          command: global-w
        - key: F
          command: global-f
`)
	writeConfig(t, directory, "shared.yml", `
keybindings:
    prs:
        - key: U
          command: included-u
`)
	repository := t.TempDir()
	writeConfig(t, repository, ".gh-dash.yml", `
include:
    - `+filepath.Join(directory, "shared.yml")+`
prSections:
    - title: Repo
      filters: is:open label:bug
keybindings:
    prs:
        - key: W
          command: repo-w
`)

	config, err := Load("", repository)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.PRSections) != 1 || config.PRSections[0].Title != "Repo" {
		t.Errorf("repo sections should replace global ones: %+v", config.PRSections)
	}
	commands := map[string]string{}
	for _, binding := range config.Keybindings.Prs {
		commands[binding.Key] = binding.Command
	}
	want := map[string]string{"W": "repo-w", "F": "global-f", "U": "included-u"}
	for key, command := range want {
		if commands[key] != command {
			t.Errorf("key %s = %q, want %q", key, commands[key], command)
		}
	}
}

func TestDefaultsWithoutAnyFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GH_DASH_CONFIG", "")
	config, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(config.PRSections) != 3 || config.Defaults.View != PRsView {
		t.Errorf("defaults: %+v", config)
	}
}

func TestRepoPath(t *testing.T) {
	t.Setenv("HOME", "/home/someone")
	config := Config{RepoPaths: map[string]string{
		"acme/core":    "~/coding/acme/core",
		"charm/*":      "~/charm/*",
		":owner/:repo": "~/coding/:repo",
	}}
	cases := map[string]string{
		"acme/core":       "/home/someone/coding/acme/core",
		"charm/bubbletea": "/home/someone/charm/bubbletea",
		"chrismatix/grog": "/home/someone/coding/grog",
	}
	for repository, want := range cases {
		if got, _ := config.RepoPath(repository); got != want {
			t.Errorf("RepoPath(%s) = %s, want %s", repository, got, want)
		}
	}
	if _, ok := (Config{}).RepoPath("a/b"); ok {
		t.Error("expected no path without repoPaths")
	}
}
