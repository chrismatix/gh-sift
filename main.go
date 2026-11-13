// gh-sift is a terminal UI for reviewing GitHub pull requests: a gh-dash
// compatible list, a PR view and a diff view in one program.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/chrismatix/gh-sift/internal/config"
	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/ui"
)

// A PR is named by its number, as owner/repo#number, or by any github.com URL
// of it, including the ones copied from a tab (".../pull/12/files"). Anything
// else on the command line is taken as a branch name.
var pullRequestReference = regexp.MustCompile(`^(?:https?://github\.com/)?(?:([\w.-]+/[\w.-]+)(?:#|/pull/))?(\d+)(?:/(\w+))?(?:[/?#].*)?$`)

func parseTarget(target, currentRepository string) (repository string, number int, tab string, err error) {
	match := pullRequestReference.FindStringSubmatch(target)
	repository = match[1]
	if repository == "" {
		repository = currentRepository
	}
	if repository == "" {
		return "", 0, "", fmt.Errorf("not inside a repository: name the PR as owner/repo#%s", match[2])
	}
	number, _ = strconv.Atoi(match[2])
	return repository, number, match[3], nil
}

func main() {
	configPath := flag.String("config", "", "config file layered over gh-dash's global one")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gh sift [--config file] [number | owner/repo#number | PR url | branch | HEAD]")
		flag.PrintDefaults()
	}
	flag.Parse()

	if err := run(*configPath, flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "gh sift:", err)
		os.Exit(1)
	}
}

func run(configPath, target string) error {
	options := ui.Options{}
	// Outside a repository both stay empty, which only disables smart filtering.
	if current, err := repository.Current(); err == nil {
		options.Repository = current.Owner + "/" + current.Name
	}
	if root, err := exec.Command("git", "rev-parse", "--show-toplevel").Output(); err == nil {
		options.RepositoryPath = strings.TrimSpace(string(root))
	}

	var err error
	if options.Config, err = config.Load(configPath, options.RepositoryPath); err != nil {
		return err
	}
	if options.Client, err = gh.NewClient(); err != nil {
		return err
	}

	switch {
	case target == "":
	case pullRequestReference.MatchString(target):
		if options.OpenRepository, options.OpenNumber, options.OpenTab, err = parseTarget(target, options.Repository); err != nil {
			return err
		}
	default:
		branch := target
		// HEAD is git's own name for the branch you are on.
		if target == "HEAD" {
			current, _ := exec.Command("git", "branch", "--show-current").Output()
			branch = strings.TrimSpace(string(current))
		}
		if options.Repository == "" || branch == "" {
			return fmt.Errorf("%q names a branch, which needs a checkout of a GitHub repository on a branch", target)
		}
		options.OpenRepository = options.Repository
		if options.OpenNumber, err = options.Client.PullRequestForBranch(options.Repository, branch); err != nil {
			return err
		}
	}

	app, err := ui.New(options)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(app).Run()
	return err
}
