package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/chrismatix/gh-sift/internal/gh"
)

// network tracks failed loads, so background polling backs off exponentially
// instead of retrying every tick, and the status chip can say why.
type network struct {
	failures  int
	retryAt   time.Time
	lastError string
}

func (state *network) observe(err error, health gh.Health) {
	if err == nil {
		*state = network{}
		return
	}
	state.failures++
	state.retryAt = time.Now().Add(min(5*time.Second<<min(state.failures-1, 6), 5*time.Minute))
	if health.BlockedUntil.After(state.retryAt) {
		state.retryAt = health.BlockedUntil
	}
	// go-gh appends the request URL, which the chip has no room for.
	state.lastError, _, _ = strings.Cut(firstLine(err), " (https://")
}

// loadError picks the outcome out of the messages that carry fetched data.
func loadError(message tea.Msg) (error, bool) {
	switch message := message.(type) {
	case sectionLoadedMsg:
		return message.err, true
	case pullRequestLoadedMsg:
		return message.err, true
	case diffLoadedMsg:
		return message.err, true
	case jobDetailMsg:
		return message.err, true
	case jobLogMsg:
		return message.err, true
	}
	return nil, false
}

// statusChip says, at the top right, when what is on screen may not be
// current: GitHub refusing, loads failing, data gone stale, or a budget running out.
func (app *App) statusChip() string {
	theme := app.theme
	health := app.client.Health()
	if time.Now().Before(health.BlockedUntil) {
		return theme.failure().Render("● rate limited · resumes " + health.BlockedUntil.Local().Format("15:04"))
	}
	if app.network.failures > 0 {
		retry := "retrying"
		if wait := time.Until(app.network.retryAt); wait > 0 {
			retry = "retry in " + wait.Round(time.Second).String()
		}
		return theme.failure().Render("● " + truncate(app.network.lastError, 40) + " · " + retry)
	}
	if synced, staleAfter := app.synced(); !synced.IsZero() && staleAfter > 0 && time.Since(synced) > staleAfter {
		return theme.warning().Render("● stale · synced " + ago(synced))
	}
	if budget := health.GraphQL; budget.Limit > 0 && budget.Remaining*10 < budget.Limit {
		return theme.warning().Render(fmt.Sprintf("● GraphQL %d/%d left · resets %s", budget.Remaining, budget.Limit, budget.Reset.Local().Format("15:04")))
	}
	return ""
}

// synced is when the data on screen was fetched, and how old it may get
// before that is worth saying: a few missed refreshes.
func (app *App) synced() (time.Time, time.Duration) {
	if app.screen == screenPullRequest {
		return app.data(app.pullRequest.key).detailAt, time.Minute
	}
	if app.config.Defaults.RefetchIntervalMinutes == 0 {
		return time.Time{}, 0
	}
	return app.list.current().loadedAt, time.Duration(app.config.Defaults.RefetchIntervalMinutes)*time.Minute + time.Minute
}
