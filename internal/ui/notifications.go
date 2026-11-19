package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/readstate"
)

func donePath() string {
	return filepath.Join(filepath.Dir(readstate.DefaultPath()), "done.json")
}

func (list *listScreen) handleNotificationKey(app *App, action string, notification *gh.Notification) tea.Cmd {
	switch action {
	case "toggleSmartFiltering":
		return list.toggleSmartFiltering(app)
	case "switchToPRs":
		return list.switchView(app)
	case "markAllAsRead":
		// GitHub has no way back to unread, so this asks first, unlike gh-dash.
		app.openConfirm("Mark every notification on GitHub as read? (y/N)", func(app *App) tea.Cmd {
			return app.run("marking all read", app.client.MarkAllNotificationsRead, "marked all notifications read", func(app *App) tea.Cmd {
				for _, current := range app.list.notificationSections {
					for index := range current.notifications {
						current.notifications[index].Unread = false
					}
				}
				return nil
			})
		})
		return nil
	case "markAllAsDone":
		threads := slices.Clone(list.current().notifications)
		if len(threads) == 0 {
			return nil
		}
		app.openConfirm(fmt.Sprintf("Mark the %s of this section as done? (y/N)", plural(len(threads), "notification")), func(app *App) tea.Cmd {
			return app.run("marking done", func() error {
				for _, thread := range threads {
					if err := app.client.MarkNotificationDone(thread.ID); err != nil {
						return err
					}
				}
				return nil
			}, "marked "+plural(len(threads), "notification")+" done", func(app *App) tea.Cmd { return app.list.markedDone(app, threads) })
		})
		return nil
	}
	if notification == nil {
		return nil
	}
	// A copy, since the rows can change before a write comes back.
	thread := *notification
	switch action {
	case "view":
		if thread.Type == "PullRequest" {
			return tea.Batch(list.markRead(app, thread), app.openPullRequest(pullRequestKey{thread.Repository, thread.Number}, nil, tabOverview))
		}
		return tea.Batch(list.markRead(app, thread), app.openInBrowser(thread.URL))
	case "open":
		return tea.Batch(list.markRead(app, thread), app.openInBrowser(thread.URL))
	case "markAsRead":
		return tea.Batch(list.markRead(app, thread), list.move(app, 1))
	case "markAsDone":
		return app.run("marking done", func() error { return app.client.MarkNotificationDone(thread.ID) }, "marked done",
			func(app *App) tea.Cmd { return app.list.markedDone(app, []gh.Notification{thread}) })
	case "unsubscribe":
		return app.run("unsubscribing", func() error { return app.client.Unsubscribe(thread.ID) }, "unsubscribed from "+thread.Title, nil)
	}
	app.notify(action+" is not supported yet", false)
	return nil
}

// markRead marks a thread read on GitHub, as opening it there would.
func (list *listScreen) markRead(app *App, thread gh.Notification) tea.Cmd {
	if !thread.Unread {
		return nil
	}
	return app.run("marking read", func() error { return app.client.MarkNotificationRead(thread.ID) }, "", func(app *App) tea.Cmd {
		for _, current := range app.list.notificationSections {
			for index := range current.notifications {
				if current.notifications[index].ID == thread.ID {
					current.notifications[index].Unread = false
				}
			}
		}
		return nil
	})
}

// markedDone takes threads GitHub marked done off every section and
// remembers them, since GitHub keeps listing them with all=true.
func (list *listScreen) markedDone(app *App, threads []gh.Notification) tea.Cmd {
	for _, thread := range threads {
		list.done[thread.ID] = thread.UpdatedAt
	}
	for _, current := range list.notificationSections {
		current.notifications = slices.DeleteFunc(current.notifications, func(row gh.Notification) bool {
			return slices.ContainsFunc(threads, func(thread gh.Notification) bool { return thread.ID == row.ID })
		})
		current.totalCount = len(current.notifications)
		current.cursor = clamp(current.cursor, 0, max(current.count()-1, 0))
	}
	if contents, err := json.Marshal(list.done); err == nil && os.MkdirAll(filepath.Dir(donePath()), 0o755) == nil {
		_ = os.WriteFile(donePath(), contents, 0o644)
	}
	return list.schedulePreview(app)
}

func (list *listScreen) notificationColumns(app *App, current *section) []column {
	theme := app.theme
	rows := current.notifications
	compact := app.config.Theme != nil && app.config.Theme.Ui.Table.Compact
	single := func(render func(row gh.Notification) string) func(int) []string {
		return func(index int) []string { return []string{render(rows[index])} }
	}
	return []column{
		{"", 3, single(func(row gh.Notification) string {
			if row.Unread {
				return theme.text(theme.Focus).Render(glyphUnread)
			}
			return ""
		})},
		{"", 3, single(func(row gh.Notification) string { return theme.notificationGlyph(row.Type) })},
		{"Title", 0, func(index int) []string {
			row := rows[index]
			where := row.Repository
			if row.Number > 0 {
				where += fmt.Sprintf(" #%d", row.Number)
			}
			title := theme.primary()
			if row.Unread {
				title = theme.bold()
			}
			if compact {
				return []string{theme.secondary().Render(where+" ") + title.Render(row.Title)}
			}
			return []string{theme.secondary().Render(where), title.Render(row.Title)}
		}},
		{"Reason", 18, single(func(row gh.Notification) string {
			return theme.secondary().Render(strings.ReplaceAll(row.Reason, "_", " "))
		})},
		{glyphUpdated, 5, single(func(row gh.Notification) string { return theme.faint().Render(elapsed(row.UpdatedAt)) })},
	}
}
