package ui

import (
	"testing"

	"github.com/chrismatix/gh-sift/internal/gh/ghtest"
)

// prStacked returns PRs #10, #11 and #12 stacked bottom to top on main.
func prStacked() []map[string]any {
	entry := func(position, number int) map[string]any {
		return map[string]any{"position": position, "pullRequest": map[string]any{
			"number": number, "title": prDetail(number)["title"], "state": "OPEN",
			"commits": prNodes(map[string]any{"commit": map[string]any{"statusCheckRollup": map[string]any{"state": "SUCCESS"}}}),
		}}
	}
	stack := map[string]any{"id": "S1", "number": 1, "size": 3, "baseRefName": "main", "entries": prNodes(entry(1, 10), entry(2, 11), entry(3, 12))}
	var details []map[string]any
	for position, number := range []int{10, 11, 12} {
		detail := prDetail(number)
		detail["stackEntry"] = map[string]any{"position": position + 1, "stack": stack}
		details = append(details, detail)
	}
	return details
}

func TestStackRailAndNavigation(t *testing.T) {
	details := prStacked()
	// Open the middle PR.
	app := prTestApp(t, details[1], details[0], details[2])
	app.expectScreen("stack 2/3", "Stack · 2 of 3", "#12 Change number 12", "▲ top", "▸ 2", "#10 Change number 10", "▼ merges first", "└ main", "K up the stack · J down · m merge")
	// The neighbours are fetched ahead.
	for _, number := range []float64{10, 12} {
		found := false
		for _, request := range app.github.Find("pendingReviews:") {
			found = found || request.Variables["number"] == number
		}
		if !found {
			t.Errorf("neighbour #%v was not prefetched", number)
		}
	}

	app.press("K")
	if app.pullRequest.key.number != 12 {
		t.Fatalf("K should move up to #12, at #%d", app.pullRequest.key.number)
	}
	app.expectScreen("stack 3/3", "Change number 12")
	app.press("K")
	app.expectScreen("top of the stack")

	app.press("J", "J")
	if app.pullRequest.key.number != 10 {
		t.Fatalf("J J should move down to #10, at #%d", app.pullRequest.key.number)
	}
	app.press("J")
	app.expectScreen("bottom of the stack")
}

func TestStackMovesKeepTheTabAndShowTheBadgeInTheDiff(t *testing.T) {
	details := prStacked()
	app := prTestApp(t, details[0], details[1], details[2])
	app.github.REST("GET", "repos/o/r/pulls/", ghtest.Reply(200, "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n"))
	app.press("4")
	app.expectScreen("#10 Change number 10", "1/3")
	app.press("K")
	if app.pullRequest.tab != tabFiles || app.pullRequest.key.number != 11 {
		t.Fatalf("moving up should stay on the diff, at #%d tab %d", app.pullRequest.key.number, app.pullRequest.tab)
	}
	app.expectScreen("#11 Change number 11", "2/3")
}

func TestStackMoveOutsideAStack(t *testing.T) {
	app := prTestApp(t, prDetail(7))
	app.press("K")
	app.expectScreen("this PR is not part of a stack")
	if stackBadge(1, nil) != "" {
		t.Error("a PR outside a stack has no badge")
	}
}

func TestStackMerge(t *testing.T) {
	tests := []struct {
		name   string
		open   int // index into prStacked, or -1 for a single PR
		change func(detail map[string]any)
		menu   []string
		keys   string
		call   string
		toast  string
	}{
		{name: "single PR squash", open: -1, menu: []string{"Merge #7", "Squash and merge", "Create a merge commit", "Rebase and merge"},
			call: "o/r pr merge 7 --squash", toast: "merged #7"},
		{name: "single PR rebase", open: -1, keys: "jj", call: "o/r pr merge 7 --rebase", toast: "merged #7"},
		{name: "auto-merge while checks run", open: -1,
			change: func(detail map[string]any) {
				detail["commits"] = prNodes(map[string]any{"commit": map[string]any{"statusCheckRollup": map[string]any{"state": "PENDING"}}})
			},
			menu: []string{"Auto-merge when checks pass"}, call: "o/r pr merge 7 --squash --auto", toast: "auto-merge enabled for #7"},
		{name: "merge queue", open: -1, change: func(detail map[string]any) { detail["isMergeQueueEnabled"] = true },
			menu: []string{"Add to the merge queue"}, call: "o/r pr merge 7", toast: "added #7 to the merge queue"},
		{name: "stack up to here", open: 1, menu: []string{"Merge stack · #11 is 2 of 3", "#11 and the 1 PR below", "the whole stack, all 3 PRs up to #12"},
			call: "o/r stack merge 11 --yes --squash", toast: "merged the stack up to #11"},
		{name: "whole stack", open: 1, keys: "jjj", call: "o/r stack merge 12 --yes --squash", toast: "merged the stack up to #12"},
		{name: "bottom of the stack", open: 0, menu: []string{"only #10, the bottom of the stack"}, call: "o/r stack merge 10 --yes --squash", toast: "merged the stack up to #10"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := prFakeGh(t)
			details := []map[string]any{prDetail(7)}
			if test.open >= 0 {
				stacked := prStacked()
				details = append([]map[string]any{stacked[test.open]}, stacked...)
			}
			if test.change != nil {
				test.change(details[0])
			}
			app := prTestApp(t, details...)
			app.press("m")
			app.expectScreen(test.menu...)
			app.press(test.keys, "enter")
			if got := prCalls(t, calls); got != test.call {
				t.Errorf("gh ran %q, want %q", got, test.call)
			}
			if app.toast.text != test.toast || app.toast.failure {
				t.Errorf("toast = %q (failure %v), want %q", app.toast.text, app.toast.failure, test.toast)
			}
		})
	}
}

func TestStackMergeIsRefused(t *testing.T) {
	merged := prDetail(7)
	merged["state"] = "MERGED"
	app := prTestApp(t, merged)
	app.press("m")
	app.expectScreen("#7 is already merged")

	closed := prDetail(8)
	closed["state"] = "CLOSED"
	app = prTestApp(t, closed)
	app.press("m")
	app.expectScreen("#8 is closed")

	app = prTestApp(t, prDetail(9))
	app.github.GraphQL("pendingReviews:", ghtest.Reply(200, map[string]any{"repository": map[string]any{"pullRequest": prDetail(9)}}))
	app.press("R", "m")
	app.expectScreen("this repository allows no merge method")
	if app.overlay != nil {
		t.Error("no menu without a merge method")
	}

	prFakeGh(t)
	app = prTestApp(t, prDetail(7))
	app.client.ReadOnly = true
	app.press("m", "enter")
	if !app.toast.failure || app.toast.text != "read-only mode (SIFT_READ_ONLY is set)" {
		t.Errorf("toast = %q, want the read-only refusal", app.toast.text)
	}
}

func TestAFetchedStackReachesTheOtherPRsInIt(t *testing.T) {
	details := prStacked()
	app := prTestApp(t, details[1], details[0], details[2])
	app.expectScreen("#12 Change number 12")

	// GitHub now reports #12 renamed, but only #10's refetch has seen it.
	stack := details[0]["stackEntry"].(map[string]any)["stack"].(map[string]any)
	renamed := map[string]any{}
	for name, value := range stack {
		renamed[name] = value
	}
	entries := stack["entries"].(map[string]any)["nodes"].([]any)
	top := map[string]any{"position": 3, "pullRequest": map[string]any{"number": 12, "title": "Renamed on GitHub", "state": "OPEN"}}
	renamed["entries"] = prNodes(entries[0], entries[1], top)
	details[0]["stackEntry"] = map[string]any{"position": 1, "stack": renamed}
	app.run(app.loadPullRequest(pullRequestKey{prRepository, 10}, true))

	app.expectScreen("#12 Renamed on GitHub")
	if title := app.data(pullRequestKey{prRepository, 12}).detail.Stack.Entries[2].Title; title != "Renamed on GitHub" {
		t.Errorf("#12's cached stack still shows %q", title)
	}
}
