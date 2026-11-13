package gh

import (
	"testing"
	"time"
)

const sampleLog = "\uFEFF2026-11-13T12:00:01.1000000Z Current runner version: '2.300.0'\n" +
	"2026-11-13T12:00:02.0000000Z ##[group]Runner Image\n" +
	"2026-11-13T12:00:02.1000000Z Image: ubuntu-24.04\n" +
	"2026-11-13T12:00:02.2000000Z ##[endgroup]\n" +
	"2026-11-13T12:00:05.0000000Z ##[group]Run make test\n" +
	"2026-11-13T12:00:05.1000000Z \x1b[36;1mmake test\x1b[0m\n" +
	"2026-11-13T12:00:05.2000000Z ##[endgroup]\n" +
	"2026-11-13T12:00:06.0000000Z [command]/usr/bin/make test\n" +
	"2026-11-13T12:00:07.0000000Z ##[warning]deprecated flag\n" +
	"2026-11-13T12:00:08.0000000Z \x1b[2Kok\tpkg\n" +
	"2026-11-13T12:00:09.0000000Z ##[error]Process completed with exit code 1.\n" +
	"\n"

func TestParseLog(t *testing.T) {
	lines := ParseLog(sampleLog)
	type expectation struct {
		text  string
		kind  LogKind
		depth int
	}
	want := []expectation{
		{"Current runner version: '2.300.0'", LogText, 0},
		{"Runner Image", LogGroup, 0},
		{"Image: ubuntu-24.04", LogText, 1},
		{"", LogText, 0},
		{"Run make test", LogStep, 0},
		{"\x1b[36;1mmake test\x1b[0m", LogText, 1},
		{"", LogText, 0},
		{"/usr/bin/make test", LogCommand, 0},
		{"deprecated flag", LogWarning, 0},
		// Cursor movement is dropped, colour is kept, tabs become spaces.
		{"ok    pkg", LogText, 0},
		{"Process completed with exit code 1.", LogError, 0},
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines, want %d: %+v", len(lines), len(want), lines)
	}
	for index, expected := range want {
		got := lines[index]
		if got.Text != expected.text || got.Kind != expected.kind || got.Depth != expected.depth {
			t.Errorf("line %d = {%q %v %d}, want {%q %v %d}", index, got.Text, got.Kind, got.Depth, expected.text, expected.kind, expected.depth)
		}
	}
	if stamp := lines[4].Time; !stamp.Equal(time.Date(2026, 11, 13, 12, 0, 5, 0, time.UTC)) {
		t.Errorf("timestamps are what places a step in the log, got %v", stamp)
	}
}
