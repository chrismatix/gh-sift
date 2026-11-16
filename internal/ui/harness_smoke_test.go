package ui

import "testing"

func TestHarnessStartsOnTheList(t *testing.T) {
	app := newTestApp(t, Options{})
	app.expectScreen("My Pull Requests")
	app.press("j", "?")
	if len(app.github.Find("search(")) == 0 {
		t.Error("the list should have searched")
	}
}
