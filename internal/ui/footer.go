package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/chrismatix/gh-sift/internal/config"
)

func (app *App) footer() string {
	theme := app.theme
	separator := theme.text(theme.PrimaryBorder).Render(" │ ")

	var left string
	switch {
	case app.overlay != nil && app.overlay.kind == overlayConfirm:
		left = " " + theme.warning().Bold(true).Render(app.overlay.title) + " "
	case app.screen == screenPullRequest && app.pullRequest.tab == tabChecks:
		left = " " + app.pullRequest.checks.summary(theme) + theme.faint().Render("  ·  "+app.pullRequest.hints(app))
	case app.screen == screenPullRequest && app.pullRequest.tab == tabFiles && (app.pullRequest.diff.typing || app.pullRequest.diff.query != ""):
		search := app.pullRequest.diff.query
		if app.pullRequest.diff.typing {
			search += "▏"
		}
		left = " " + theme.text(theme.Focus).Render("search ") + search + theme.faint().Render("  ·  "+app.pullRequest.hints(app))
	case app.screen == screenPullRequest:
		left = " " + theme.faint().Render(app.pullRequest.hints(app))
	default:
		active, inactive := theme.success().Bold(true), theme.faint()
		pullRequests, issues := inactive, inactive
		if app.list.kind == config.PRsView {
			pullRequests = active
		} else {
			issues = active
		}
		left = " " + pullRequests.Render(glyphOpenPR+" PRs") + separator + issues.Render(glyphOpenIssue+" Issues")
		if app.repository != "" {
			left += separator + theme.secondary().Render(glyphRepo+" "+app.repository)
		}
		if app.viewer != "" {
			left += theme.faint().Render(" • @" + app.viewer)
		}
		current := app.list.current()
		if count := current.count(); count > 0 {
			left += separator + theme.faint().Render(fmt.Sprintf("%d/%d", current.cursor+1, current.totalCount))
			if !current.loadedAt.IsZero() {
				left += theme.faint().Render(" • updated " + ago(current.loadedAt))
			}
		}
	}

	right := ""
	switch {
	case app.busy > 0:
		right = theme.warning().Render(app.spinner() + " " + app.busyText)
	case app.toast.text != "" && time.Now().Before(app.toast.until):
		style := theme.success()
		if app.toast.failure {
			style = theme.failure()
		}
		right = style.Render(app.toast.text)
	case app.client.ReadOnly:
		right = theme.warning().Render("read-only")
	}
	help := lipgloss.NewStyle().Foreground(theme.SelectedBackground).Background(theme.FaintText).Padding(0, 1).Render("? help")
	right = truncate(right, max(app.width-width(left)-12, 10)) + "  " + help

	return paintBackground(spread(left, right, app.width), theme.SelectedBackground)
}

func (app *App) helpBox() string {
	theme := app.theme
	type group struct {
		title   string
		entries []helpEntry
	}
	var groups []group
	switch {
	case app.screen == screenPullRequest && app.pullRequest.tab == tabFiles:
		half := (len(diffHelp) + 1) / 2
		groups = []group{{"Diff", diffHelp[:half]}, {"", diffHelp[half:]}}
	case app.screen == screenPullRequest && app.pullRequest.tab == tabChecks:
		half := (len(checksHelp) + 1) / 2
		groups = []group{{"Checks", checksHelp[:half]}, {"", checksHelp[half:]}}
	case app.screen == screenPullRequest:
		half := (len(pullRequestHelp) + 1) / 2
		groups = []group{{"Pull request", pullRequestHelp[:half]}, {"", pullRequestHelp[half:]}}
	case app.list.kind == config.IssuesView:
		groups = []group{{"Navigation and app", helpFor(app.keys.universal, app.keys.customUniversal...)}, {"Issues", helpFor(app.keys.issues, app.keys.customIssues...)}}
		if custom := append(helpForCustom(app.keys.customIssues), helpForCustom(app.keys.customUniversal)...); len(custom) > 0 {
			groups = append(groups, group{"Custom", custom})
		}
	default:
		groups = []group{{"Navigation and app", helpFor(app.keys.universal, app.keys.customUniversal...)}, {"Pull requests", helpFor(app.keys.prs, app.keys.customPRs...)}}
		if custom := append(helpForCustom(app.keys.customPRs), helpForCustom(app.keys.customUniversal)...); len(custom) > 0 {
			groups = append(groups, group{"Custom", custom})
		}
	}

	var columns []string
	for _, each := range groups {
		lines := []string{theme.bold().Render(each.title), ""}
		for _, entry := range each.entries {
			lines = append(lines, theme.text(theme.Focus).Render(fit(entry.keys, 14))+theme.secondary().Render(entry.help))
		}
		columns = append(columns, lipgloss.NewStyle().PaddingRight(3).Render(strings.Join(lines, "\n")))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).BorderForeground(theme.PrimaryBorder).
		Padding(1, 2).MaxWidth(app.width - 2).MaxHeight(app.height - 2).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, columns...))
}
