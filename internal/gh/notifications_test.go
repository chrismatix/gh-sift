package gh_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/chrismatix/gh-sift/internal/gh"
	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

func notificationNode(id string, unread bool, reason, subjectType, subjectURL string) map[string]any {
	return map[string]any{
		"id": id, "unread": unread, "reason": reason, "updated_at": "2026-11-18T10:00:00Z",
		"subject":    map[string]any{"title": "Title of " + id + "\r\n", "url": subjectURL, "type": subjectType},
		"repository": map[string]any{"full_name": "o/r", "html_url": "https://github.com/o/r"},
	}
}

func notificationNodes() []any {
	return []any{
		notificationNode("1", true, "review_requested", "PullRequest", "https://api.github.com/repos/o/r/pulls/11"),
		notificationNode("2", false, "mention", "Issue", "https://api.github.com/repos/o/r/issues/12"),
		notificationNode("3", true, "subscribed", "Release", "https://api.github.com/repos/o/r/releases/99"),
		notificationNode("4", false, "author", "PullRequest", "https://api.github.com/repos/o/r/pulls/14"),
	}
}

func notificationIDs(rows []gh.Notification) []string {
	ids := []string{}
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	return ids
}

func TestNotificationFilters(t *testing.T) {
	tests := []struct {
		query       string
		includeRead bool
		path        string
		ids         []string
	}{
		{"", true, "notifications?per_page=50&page=1&all=true", []string{"1", "2", "3", "4"}},
		{"", false, "notifications?per_page=50&page=1&all=false", []string{"1", "3"}},
		{"is:unread", true, "notifications?per_page=50&page=1&all=false", []string{"1", "3"}},
		{"is:read", false, "notifications?per_page=50&page=1&all=true", []string{"2", "4"}},
		{"is:read is:unread", false, "notifications?per_page=50&page=1&all=true", []string{"1", "2", "3", "4"}},
		{"is:all", false, "notifications?per_page=50&page=1&all=true", []string{"1", "2", "3", "4"}},
		{"reason:participating", true, "notifications?", []string{"1", "2", "4"}},
		{"reason:review-requested reason:subscribed", true, "notifications?", []string{"1", "3"}},
		{"repo:o/r reason:mention", true, "repos/o/r/notifications?per_page=50&page=1&all=true", []string{"2"}},
	}
	for _, test := range tests {
		t.Run(test.query, func(t *testing.T) {
			server, client := ghtest.New(t)
			server.REST("GET", "", ghtest.Reply(200, notificationNodes()))
			page, err := client.Notifications(test.query, test.includeRead, nil, 20, "")
			if err != nil {
				t.Fatal(err)
			}
			if ids := notificationIDs(page.Rows); !reflect.DeepEqual(ids, test.ids) {
				t.Errorf("kept %v, want %v", ids, test.ids)
			}
			if len(server.Requests) != 1 || !strings.HasPrefix(server.Requests[0].Path, test.path) {
				t.Errorf("requested %+v, want %s", server.Requests, test.path)
			}
			if page.TotalCount != len(test.ids) || page.HasNextPage {
				t.Errorf("page = %+v", page)
			}
		})
	}
}

func TestNotificationDoneFilterAsksNothing(t *testing.T) {
	server, client := ghtest.New(t)
	page, err := client.Notifications("is:done", true, nil, 20, "")
	if err != nil || len(page.Rows) != 0 || len(server.Requests) != 0 {
		t.Errorf("is:done should list nothing without asking: %+v %v %+v", page, err, server.Requests)
	}
}

func TestNotificationsSeveralRepositories(t *testing.T) {
	server, client := ghtest.New(t)
	server.REST("GET", "repos/o/r/", ghtest.Reply(200, notificationNodes()[:1]))
	server.REST("GET", "repos/o/s/", ghtest.Reply(200, notificationNodes()[1:2]))
	page, err := client.Notifications("repo:o/r repo:o/s", true, nil, 20, "")
	if err != nil || !reflect.DeepEqual(notificationIDs(page.Rows), []string{"1", "2"}) {
		t.Errorf("both repositories should be read: %+v %v", page, err)
	}
}

func TestNotificationRows(t *testing.T) {
	server, client := ghtest.New(t)
	nodes := append(notificationNodes(),
		notificationNode("5", true, "comment", "Discussion", ""),
		notificationNode("6", true, "comment", "Commit", "https://api.github.com/repos/o/r/commits/abc"),
		notificationNode("7", true, "ci_activity", "CheckSuite", ""),
		notificationNode("8", true, "security_alert", "RepositoryVulnerabilityAlert", ""),
	)
	// Without html_url the address is built from the name.
	nodes[7].(map[string]any)["repository"] = map[string]any{"full_name": "o/r"}
	server.REST("GET", "notifications", ghtest.Reply(200, nodes))
	page, err := client.Notifications("", true, nil, 20, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		number int
		url    string
	}{
		{11, "https://github.com/o/r/pull/11"},
		{12, "https://github.com/o/r/issues/12"},
		{0, "https://github.com/o/r/releases"},
		{14, "https://github.com/o/r/pull/14"},
		{0, "https://github.com/o/r/discussions"},
		{0, "https://github.com/o/r/commits"},
		{0, "https://github.com/o/r/actions"},
		{0, "https://github.com/o/r"},
	}
	for index, row := range page.Rows {
		if row.Number != want[index].number || row.URL != want[index].url {
			t.Errorf("row %s: #%d %s, want #%d %s", row.ID, row.Number, row.URL, want[index].number, want[index].url)
		}
	}
	first := page.Rows[0]
	if first.Title != "Title of 1" || first.Repository != "o/r" || first.Reason != "review_requested" || !first.Unread ||
		!first.UpdatedAt.Equal(time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)) {
		t.Errorf("first row = %+v", first)
	}
}

func TestNotificationPagesFillTheLimit(t *testing.T) {
	server, client := ghtest.New(t)
	var full []any
	for index := range 50 {
		full = append(full, notificationNode(fmt.Sprint(100+index), true, "subscribed", "Issue", ""))
	}
	server.REST("GET", "notifications?per_page=50&page=2", ghtest.Reply(200, full))
	server.REST("GET", "notifications?per_page=50&page=3", ghtest.Reply(200, notificationNodes()))

	// Page 2 has no mention, so page 3 is read too, and it is the last.
	page, err := client.Notifications("reason:mention", true, nil, 20, "2")
	if err != nil || !reflect.DeepEqual(notificationIDs(page.Rows), []string{"2"}) || page.HasNextPage || page.EndCursor != "4" {
		t.Errorf("page = %+v, %v", page, err)
	}

	// A full page that already has enough rows stops there.
	page, err = client.Notifications("", true, nil, 20, "2")
	if err != nil || len(page.Rows) != 50 || !page.HasNextPage || page.EndCursor != "3" {
		t.Errorf("page = %d rows, next %v %q, %v", len(page.Rows), page.HasNextPage, page.EndCursor, err)
	}

	server.REST("GET", "notifications?per_page=50&page=2", ghtest.Reply(500, map[string]any{"message": "boom"}))
	if _, err := client.Notifications("", true, nil, 20, "2"); err == nil {
		t.Error("a failed page should fail the load")
	}
}

func TestNotificationsDoneStayHidden(t *testing.T) {
	server, client := ghtest.New(t)
	server.REST("GET", "notifications", ghtest.Reply(200, notificationNodes()))
	updated := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	done := map[string]time.Time{"1": updated, "2": updated.Add(-time.Hour)}
	page, err := client.Notifications("", true, done, 20, "")
	// 2 changed after it was marked done, so it is back.
	if err != nil || !reflect.DeepEqual(notificationIDs(page.Rows), []string{"2", "3", "4"}) {
		t.Errorf("page = %+v, %v", page, err)
	}
}

func TestNotificationWrites(t *testing.T) {
	tests := []struct {
		name   string
		call   func(client *gh.Client) error
		method string
		path   string
	}{
		{"read", func(client *gh.Client) error { return client.MarkNotificationRead("7") }, "PATCH", "notifications/threads/7"},
		{"done", func(client *gh.Client) error { return client.MarkNotificationDone("7") }, "DELETE", "notifications/threads/7"},
		{"all read", func(client *gh.Client) error { return client.MarkAllNotificationsRead() }, "PUT", "notifications"},
		{"unsubscribe", func(client *gh.Client) error { return client.Unsubscribe("7") }, "DELETE", "notifications/threads/7/subscription"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, client := ghtest.New(t)
			// GitHub answers these with 205 and 204 and no body.
			server.REST(test.method, test.path, ghtest.Reply(205, ""))
			if err := test.call(client); err != nil {
				t.Fatal(err)
			}
			if len(server.Requests) != 1 || server.Requests[0].Method != test.method || server.Requests[0].Path != test.path {
				t.Errorf("sent %+v", server.Requests)
			}

			client.ReadOnly = true
			if err := test.call(client); !errors.Is(err, gh.ErrReadOnly) {
				t.Errorf("read-only mode should refuse, got %v", err)
			}
			if len(server.Requests) != 1 {
				t.Errorf("read-only mode still sent %+v", server.Requests[1:])
			}
		})
	}
}
