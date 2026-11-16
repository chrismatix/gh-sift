package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobalPathFallsBackToHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/someone")
	if got := GlobalPath(); got != "/home/someone/.config/gh-dash/config.yml" {
		t.Errorf("GlobalPath() = %s", got)
	}
}

func TestProvidedPath(t *testing.T) {
	repository := t.TempDir()
	t.Setenv("GH_DASH_CONFIG", "")
	if got := providedPath("", repository); got != "" {
		t.Errorf("a repository without a config provides none, got %q", got)
	}
	yaml := writeConfig(t, repository, ".gh-dash.yaml", "")
	if got := providedPath("", repository); got != yaml {
		t.Errorf("the .yaml spelling should be found, got %q", got)
	}
	t.Setenv("GH_DASH_CONFIG", "/from/environment.yml")
	if got := providedPath("", repository); got != "/from/environment.yml" {
		t.Errorf("$GH_DASH_CONFIG beats the repository, got %q", got)
	}
	if got := providedPath("/from/flag.yml", repository); got != "/from/flag.yml" {
		t.Errorf("--config beats everything, got %q", got)
	}
}

func TestLoadIncludes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GH_DASH_CONFIG", "")
	directory := t.TempDir()
	writeConfig(t, home, "from-home.yml", "defaults:\n    prsLimit: 11\n")
	writeConfig(t, directory, "relative.yml", "defaults:\n    issuesLimit: 12\n")
	// The cycle back to main.yml must be cut, not loaded again or looped on.
	writeConfig(t, directory, "cycle.yml", "include:\n    - main.yml\nconfirmQuit: true\n")
	main := writeConfig(t, directory, "main.yml", `
include:
    - ~/from-home.yml
    - relative.yml
    - `+filepath.Join(directory, "cycle.yml")+`
defaults:
    prsLimit: 13
`)

	config, err := Load(main, "")
	if err != nil {
		t.Fatal(err)
	}
	if config.Defaults.PrsLimit != 13 {
		t.Errorf("the including file wins over its includes, prsLimit = %d", config.Defaults.PrsLimit)
	}
	if config.Defaults.IssuesLimit != 12 || !config.ConfirmQuit {
		t.Errorf("includes should load: issuesLimit %d, confirmQuit %v", config.Defaults.IssuesLimit, config.ConfirmQuit)
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name     string
		global   string
		provided string
		message  string
	}{
		{"broken global file", "prSections: [", "", "parsing config"},
		{"global with wrong types", "prSections: 5", "", "parsing config"},
		{"missing provided file", "", "missing.yml", "reading config"},
		{"missing include", "", "include:\n    - nowhere.yml\n", "reading config"},
	}
	for _, each := range cases {
		t.Run(each.name, func(t *testing.T) {
			configHome := t.TempDir()
			t.Setenv("XDG_CONFIG_HOME", configHome)
			t.Setenv("GH_DASH_CONFIG", "")
			if each.global != "" {
				if err := os.MkdirAll(filepath.Join(configHome, "gh-dash"), 0o755); err != nil {
					t.Fatal(err)
				}
				writeConfig(t, filepath.Join(configHome, "gh-dash"), "config.yml", each.global)
			}
			provided := ""
			switch {
			case each.provided == "missing.yml":
				provided = filepath.Join(t.TempDir(), "missing.yml")
			case each.provided != "":
				provided = writeConfig(t, t.TempDir(), "config.yml", each.provided)
			}
			if _, err := Load(provided, ""); err == nil || !strings.Contains(err.Error(), each.message) {
				t.Errorf("err = %v, want %q", err, each.message)
			}
		})
	}
}

func TestRepoPathLeavesAbsolutePathsAlone(t *testing.T) {
	config := Config{RepoPaths: map[string]string{"acme/core": "/srv/core"}}
	if got, ok := config.RepoPath("acme/core"); !ok || got != "/srv/core" {
		t.Errorf("RepoPath = %s, %v", got, ok)
	}
}
