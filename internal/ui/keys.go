package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/chrismatix/gh-sift/internal/config"
)

// binding is one built-in list action. Names match gh-dash's `builtin:`
// values so a config that rebinds them keeps working.
type binding struct {
	name string
	keys []string
	help string
}

type keymap struct {
	universal       []binding
	prs             []binding
	issues          []binding
	notifications   []binding
	customUniversal []config.Keybinding
	customPRs       []config.Keybinding
	customIssues    []config.Keybinding
	// Custom notification commands, as in gh-dash, get RepoName and PrNumber or IssueNumber.
	customNotifications []config.Keybinding
}

func defaultKeymap() keymap {
	return keymap{
		universal: []binding{
			{"up", []string{"up", "k"}, "move up"},
			{"down", []string{"down", "j"}, "move down"},
			{"firstLine", []string{"g", "home"}, "first item"},
			{"lastLine", []string{"G", "end"}, "last item"},
			{"prevSection", []string{"left", "h"}, "previous section"},
			{"nextSection", []string{"right", "l"}, "next section"},
			{"pageDown", []string{"ctrl+d"}, "preview page down"},
			{"pageUp", []string{"ctrl+u"}, "preview page up"},
			{"togglePreview", []string{"p"}, "toggle preview"},
			{"togglePreviewPosition", []string{"P"}, "toggle preview position"},
			{"openGithub", []string{"o"}, "open in GitHub"},
			{"refresh", []string{"r"}, "refresh"},
			{"refreshAll", []string{"R"}, "refresh all"},
			{"redraw", nil, "redraw"},
			{"search", []string{"/"}, "search"},
			{"copyNumber", []string{"y"}, "copy number"},
			{"copyurl", []string{"Y"}, "copy url"},
			{"newSection", nil, "new section"},
			{"help", []string{"?"}, "help"},
			{"quit", []string{"q", "ctrl+c"}, "quit"},
		},
		prs: []binding{
			{"open", []string{"enter"}, "open the PR"},
			{"diff", []string{"d"}, "diff"},
			{"stackUp", []string{"K"}, "up the stack"},
			{"stackDown", []string{"J"}, "down the stack"},
			{"merge", []string{"m"}, "merge (stack aware)"},
			{"approve", []string{"v"}, "approve"},
			{"comment", []string{"c"}, "comment"},
			{"assign", []string{"a"}, "assign"},
			{"unassign", []string{"A"}, "unassign"},
			{"label", []string{"L"}, "label"},
			{"checkout", []string{"C", "space"}, "checkout"},
			{"close", []string{"x"}, "close"},
			{"reopen", []string{"X"}, "reopen"},
			{"ready", []string{"W"}, "ready for review"},
			{"update", []string{"u"}, "update pr from base branch"},
			{"toggleSmartFiltering", []string{"t"}, "toggle smart filtering"},
			{"viewIssues", []string{"s"}, "switch to issues"},
			// Accepted for config compatibility; sift has no equivalent yet.
			{"prevSidebarTab", nil, ""},
			{"nextSidebarTab", nil, ""},
			{"watchChecks", nil, ""},
			{"approveWorkflows", nil, ""},
			{"summaryViewMore", nil, ""},
		},
		issues: []binding{
			{"comment", []string{"c"}, "comment"},
			{"assign", []string{"a"}, "assign"},
			{"unassign", []string{"A"}, "unassign"},
			{"label", []string{"L"}, "label"},
			{"close", []string{"x"}, "close"},
			{"reopen", []string{"X"}, "reopen"},
			{"toggleSmartFiltering", []string{"t"}, "toggle smart filtering"},
			{"viewPrs", []string{"s"}, "switch to notifications"},
			{"checkout", nil, ""},
		},
		notifications: []binding{
			{"view", []string{"enter"}, "open (marks it read)"},
			{"markAsRead", []string{"m"}, "mark as read"},
			{"markAllAsRead", []string{"M"}, "mark all as read"},
			{"markAsDone", []string{"D"}, "mark as done"},
			{"markAllAsDone", []string{"alt+d"}, "mark all as done"},
			{"unsubscribe", []string{"u"}, "unsubscribe"},
			{"open", []string{"o"}, "open in GitHub (marks it read)"},
			{"toggleSmartFiltering", []string{"t"}, "toggle smart filtering"},
			{"switchToPRs", []string{"s"}, "switch to PRs"},
			{"toggleBookmark", nil, ""},
			{"sortByRepo", nil, ""},
		},
	}
}

func newKeymap(configured config.Keybindings) (keymap, error) {
	keys := defaultKeymap()
	var err error
	if keys.customUniversal, err = rebind(keys.universal, configured.Universal, "universal"); err != nil {
		return keys, err
	}
	if keys.customPRs, err = rebind(keys.prs, configured.Prs, "pr"); err != nil {
		return keys, err
	}
	if keys.customIssues, err = rebind(keys.issues, configured.Issues, "issue"); err != nil {
		return keys, err
	}
	keys.customNotifications, err = rebind(keys.notifications, configured.Notifications, "notification")
	return keys, err
}

// rebind applies `builtin:` entries in place and returns the custom commands.
func rebind(bindings []binding, configured []config.Keybinding, group string) ([]config.Keybinding, error) {
	var custom []config.Keybinding
	for _, entry := range configured {
		if entry.Builtin == "" {
			if entry.Command != "" {
				custom = append(custom, entry)
			}
			continue
		}
		index := slices.IndexFunc(bindings, func(candidate binding) bool { return candidate.name == entry.Builtin })
		if index < 0 {
			return nil, fmt.Errorf("unknown built-in %s key: '%s'", group, entry.Builtin)
		}
		bindings[index].keys = []string{entry.Key}
		if entry.Name != "" {
			bindings[index].help = entry.Name
		}
	}
	return custom, nil
}

func match(bindings []binding, key string) string {
	for _, candidate := range bindings {
		if slices.Contains(candidate.keys, key) {
			return candidate.name
		}
	}
	return ""
}

func matchCustom(custom []config.Keybinding, key string) *config.Keybinding {
	for index := range custom {
		if custom[index].Key == key {
			return &custom[index]
		}
	}
	return nil
}

type helpEntry struct {
	keys string
	help string
}

// helpFor lists builtins, leaving out keys a custom command has taken over.
func helpFor(bindings []binding, custom ...config.Keybinding) []helpEntry {
	var entries []helpEntry
	for _, each := range bindings {
		keys := slices.DeleteFunc(slices.Clone(each.keys), func(key string) bool { return matchCustom(custom, key) != nil })
		if len(keys) > 0 && each.help != "" {
			entries = append(entries, helpEntry{strings.Join(keys, "/"), each.help})
		}
	}
	return entries
}

func helpForCustom(custom []config.Keybinding) []helpEntry {
	var entries []helpEntry
	for _, each := range custom {
		name := each.Name
		if name == "" {
			name = truncate(strings.ReplaceAll(each.Command, "\n", " "), 30)
		}
		entries = append(entries, helpEntry{each.Key, name})
	}
	return entries
}

var pullRequestHelp = []helpEntry{
	{"tab/shift+tab", "next / previous tab"},
	{"1-4", "Overview, Checks, Threads, Files"},
	{"j/k", "move"},
	{"ctrl+d/ctrl+u", "half page"},
	{"g/G", "top / bottom"},
	{"K/J", "PR above / below in the stack"},
	{"d", "diff"},
	{"enter", "show the thread in the diff"},
	{"a", "submit a review"},
	{"m", "merge (stack aware)"},
	{"c", "comment on the PR"},
	{"r", "reply to the thread"},
	{"x", "resolve / unresolve the thread"},
	{"h", "show or hide resolved threads"},
	{"R", "refresh"},
	{"o", "open in GitHub"},
	{"y/Y", "copy number / url"},
	{"esc/q", "back"},
}

var diffHelp = []helpEntry{
	{"j/k", "line down / up"},
	{"]/[", "next / previous hunk"},
	{"./,", "next / previous file"},
	{"}/{", "next / previous thread"},
	{"d/u", "half page down / up"},
	{"space/b", "page down / up"},
	{"g/G", "top / bottom"},
	{"left/right", "scroll code sideways"},
	{"0/1/2", "layout: auto, split, stack"},
	{"s", "show or hide the files pane"},
	{"/", "jump to a file"},
	{"ctrl+f", "search the text of the diff"},
	{"n/N", "next / previous match"},
	{"x", "check off: hunk read, thread resolved"},
	{"v", "mark the file viewed on GitHub"},
	{"H", "show read hunks dimmed or folded"},
	{"z", "fold or unfold the file"},
	{"c", "comment on the line or selection"},
	{"S", "suggest a change"},
	{"V", "select lines"},
	{"r", "reply to the thread"},
	{"enter", "expand or collapse the thread"},
	{"a", "submit the review"},
	{"m", "merge (stack aware)"},
	{"K/J", "PR above / below in the stack"},
	{"tab", "next PR tab"},
	{"R", "refresh"},
	{"esc/q", "back"},
}

var checksHelp = []helpEntry{
	{"h/l", "previous / next pane"},
	{"j/k", "move in the pane"},
	{"ctrl+d/ctrl+u", "half page"},
	{"g/G", "top / bottom"},
	{"enter", "into the next pane"},
	{"z", "zoom the pane"},
	{"f", "flat list of checks"},
	{"/", "search the log"},
	{"n/N", "next / previous match"},
	{"e/E", "next / previous error"},
	{"]/[", "next / previous group"},
	{"ctrl+r", "rerun failed runs"},
	{"o/O", "open the check / the PR"},
	{"tab", "next PR tab"},
	{"K/J", "PR above / below in the stack"},
	{"R", "refresh"},
	{"m", "merge (stack aware)"},
	{"esc/q", "back"},
}
