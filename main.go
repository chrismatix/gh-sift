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

var pullRequestReference = regexp.MustCompile(`^(?:https://github\.com/)?(?:([\w.-]+/[\w.-]+)(?:#|/pull/))?(\d+)$`)

func main() {
	configPath := flag.String("config", "", "config file layered over gh-dash's global one")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gh sift [--config file] [number | owner/repo#number | PR url]")
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

	if target != "" {
		match := pullRequestReference.FindStringSubmatch(target)
		if match == nil {
			return fmt.Errorf("cannot read %q as a pull request", target)
		}
		options.OpenRepository = match[1]
		if options.OpenRepository == "" {
			options.OpenRepository = options.Repository
		}
		if options.OpenRepository == "" {
			return fmt.Errorf("not inside a repository: name the PR as owner/repo#%s", match[2])
		}
		options.OpenNumber, _ = strconv.Atoi(match[2])
	}

	var err error
	if options.Config, err = config.Load(configPath, options.RepositoryPath); err != nil {
		return err
	}
	if options.Client, err = gh.NewClient(); err != nil {
		return err
	}
	app, err := ui.New(options)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(app).Run()
	return err
}
