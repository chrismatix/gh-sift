package gh

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Notification is one thread of the viewer's inbox.
type Notification struct {
	ID         string
	Unread     bool
	Reason     string
	UpdatedAt  time.Time
	Title      string
	Type       string
	Repository string
	// Zero unless the subject is a PR or an issue.
	Number int
	URL    string
}

type rawNotification struct {
	ID        string
	Unread    bool
	Reason    string
	UpdatedAt time.Time `json:"updated_at"`
	Subject   struct {
		Title string
		URL   string
		Type  string
	}
	Repository struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
	}
}

func (raw rawNotification) convert() Notification {
	notification := Notification{
		ID: raw.ID, Unread: raw.Unread, Reason: raw.Reason, UpdatedAt: raw.UpdatedAt,
		// GitHub sends titles with stray \r and newlines, which break the row.
		Title: strings.Join(strings.Fields(raw.Subject.Title), " "),
		Type:  raw.Subject.Type, Repository: raw.Repository.FullName,
	}
	repositoryURL := strings.TrimRight(raw.Repository.HTMLURL, "/")
	if repositoryURL == "" {
		repositoryURL = "https://github.com/" + raw.Repository.FullName
	}
	// The subject's URL is an API URL ending in the number, or null.
	last := raw.Subject.URL[strings.LastIndex(raw.Subject.URL, "/")+1:]
	number, _ := strconv.Atoi(last)
	switch raw.Subject.Type {
	case "PullRequest":
		notification.Number, notification.URL = number, fmt.Sprintf("%s/pull/%d", repositoryURL, number)
	case "Issue":
		notification.Number, notification.URL = number, fmt.Sprintf("%s/issues/%d", repositoryURL, number)
	case "Discussion":
		notification.URL = strings.TrimSuffix(repositoryURL+"/discussions/"+last, "/")
	case "Release":
		notification.URL = repositoryURL + "/releases"
	case "Commit":
		notification.URL = repositoryURL + "/commits"
	case "CheckSuite":
		notification.URL = repositoryURL + "/actions"
	default:
		notification.URL = repositoryURL
	}
	return notification
}

// GitHub serves at most 50 notifications a page. The page size stays fixed
// so a page number in a cursor always means the same rows.
const notificationsPerPage = 50

var (
	repositoryFilter = regexp.MustCompile(`repo:(\S+)`)
	reasonFilter     = regexp.MustCompile(`reason:(\S+)`)
	stateFilter      = regexp.MustCompile(`is:(unread|read|done|all)`)
)

// Notifications reads the inbox the way gh-dash filters it: `repo:` picks
// the endpoint, `is:` and `reason:` are applied here because the API cannot.
// Threads in done (id → updated_at when marked) are left out until they
// change again. Pages are read until wanted rows match or none are left;
// after is the page to start from.
func (client *Client) Notifications(query string, includeRead bool, done map[string]time.Time, wanted int, after string) (Page[Notification], error) {
	states := map[string]bool{}
	for _, found := range stateFilter.FindAllStringSubmatch(query, -1) {
		states[found[1]] = true
	}
	// GitHub keeps no done threads to list.
	if states["done"] {
		return Page[Notification]{}, nil
	}
	keepRead, keepUnread := includeRead, true
	switch {
	case states["all"] || states["read"] && states["unread"]:
		keepRead, keepUnread = true, true
	case states["read"]:
		keepRead, keepUnread = true, false
	case states["unread"]:
		keepRead, keepUnread = false, true
	}

	reasons := map[string]bool{}
	for _, found := range reasonFilter.FindAllStringSubmatch(query, -1) {
		if found[1] == "participating" {
			for _, reason := range []string{"author", "comment", "mention", "review_requested", "assign", "state_change"} {
				reasons[reason] = true
			}
			continue
		}
		// Filters are written review-requested; the API says review_requested.
		reasons[strings.ReplaceAll(found[1], "-", "_")] = true
	}

	paths := []string{"notifications"}
	if found := repositoryFilter.FindAllStringSubmatch(query, -1); len(found) > 0 {
		paths = nil
		for _, each := range found {
			paths = append(paths, "repos/"+each[1]+"/notifications")
		}
	}

	page, _ := strconv.Atoi(after)
	page = max(page, 1)
	var result Page[Notification]
	for {
		full := false
		for _, path := range paths {
			var raw []rawNotification
			if err := client.rest.Get(fmt.Sprintf("%s?per_page=%d&page=%d&all=%t", path, notificationsPerPage, page, keepRead), &raw); err != nil {
				return result, err
			}
			full = full || len(raw) == notificationsPerPage
			for _, each := range raw {
				notification := each.convert()
				doneAt, isDone := done[notification.ID]
				switch {
				case isDone && !notification.UpdatedAt.After(doneAt):
				case notification.Unread && !keepUnread, !notification.Unread && !keepRead:
				case len(reasons) > 0 && !reasons[notification.Reason]:
				default:
					result.Rows = append(result.Rows, notification)
				}
			}
		}
		page++
		result.HasNextPage, result.EndCursor = full, strconv.Itoa(page)
		if !full || len(result.Rows) >= wanted {
			result.TotalCount = len(result.Rows)
			return result, nil
		}
	}
}

func (client *Client) restWrite(method, path string) error {
	if client.ReadOnly {
		return ErrReadOnly
	}
	return client.rest.Do(method, path, nil, nil)
}

func (client *Client) MarkNotificationRead(id string) error {
	return client.restWrite("PATCH", "notifications/threads/"+id)
}

// MarkNotificationDone is GitHub's "Done": the thread leaves the inbox.
func (client *Client) MarkNotificationDone(id string) error {
	return client.restWrite("DELETE", "notifications/threads/"+id)
}

func (client *Client) MarkAllNotificationsRead() error {
	return client.restWrite("PUT", "notifications")
}

// Unsubscribe mutes the thread until the viewer comments or is mentioned.
func (client *Client) Unsubscribe(id string) error {
	return client.restWrite("DELETE", "notifications/threads/"+id+"/subscription")
}
