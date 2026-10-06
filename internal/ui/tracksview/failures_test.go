package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
)

func TestFailuresShowAndDismiss(t *testing.T) {
	tracks := demoTracks(2)
	tracks[0].ID = "t1"
	tracks[0].Failures = []track.Failure{{Kind: track.ServerError, Subject: "shop/web", Code: 1}}
	tracks[0].FailureStatus = track.FailureStatus(tracks[0].Failures)
	var dismissed []string
	m := New(Config{Version: "test", Theme: theme.Default(), DismissFailures: func(id string) error {
		dismissed = append(dismissed, id)
		return nil
	}})
	m = settle(m, tea.WindowSizeMsg{Width: 140, Height: 40}, tracksMsg{tracks: tracks})
	view := plainView(m)
	if !strings.Contains(view, " server error ") {
		t.Fatal("a track with a crashed server should show the badge")
	}
	if !strings.Contains(view, "Errors    server shop/web crashed (exit 1)") {
		t.Errorf("the details should list the error:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		if strings.Contains(line, "Dismiss track errors") {
			if strings.Contains(line, "Copy session") || strings.TrimSpace(strings.Trim(lines[i-1], "│ ")) != "" {
				t.Error("Dismiss track errors should be on a row of its own, under an empty row")
			}
		}
	}
	m = clickText(t, m, "Dismiss track errors")
	if len(dismissed) != 1 || dismissed[0] != "t1" {
		t.Errorf("dismissed %v; want t1", dismissed)
	}
	m = settle(m, tea.KeyPressMsg{Code: 't', Text: "t"})
	if len(dismissed) != 2 {
		t.Errorf("t should dismiss too: %v", dismissed)
	}

	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if strings.Contains(plainView(m), "Dismiss track errors") {
		t.Error("a track without errors shouldn't offer to dismiss them")
	}
}
