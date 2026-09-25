package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// clickButton clicks the details button showing label.
func clickButton(t *testing.T, m Model, label string) Model {
	t.Helper()
	for y, line := range strings.Split(escapes.ReplaceAllString(m.View().Content, ""), "\n") {
		if x := strings.Index(line, " "+label+" "); x >= 0 && strings.Contains(line[:x], "│") {
			return run(m, tea.MouseClickMsg{X: len([]rune(line[:x])) + 1, Y: y, Button: tea.MouseLeft})
		}
	}
	t.Fatalf("button %s not drawn", label)
	return m
}

func TestEndingAsksFirst(t *testing.T) {
	var ended []int
	m := New(Config{Version: "test", Theme: theme.Default(), End: func(n int) error {
		ended = append(ended, n)
		return nil
	}})
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: demoTracks(3)})

	m = run(m, tea.KeyPressMsg{Code: 'e'})
	m = run(m, tea.KeyPressMsg{Code: 'n'})
	m = clickButton(t, m, "End")
	m = clickButton(t, m, "Cancel")
	if len(ended) != 0 || m.station.confirming {
		t.Fatalf("cancelled twice, yet ended %v (asking: %v)", ended, m.station.confirming)
	}

	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = run(m, tea.KeyPressMsg{Code: 'e'})
	m = run(m, tea.KeyPressMsg{Code: 'y'})
	m = clickButton(t, m, "End")
	m = clickButton(t, m, "End track")
	if len(ended) != 2 || ended[0] != 2 || ended[1] != 2 {
		t.Errorf("ended %v, want track 2 by key and by click", ended)
	}
}

func TestDetailsNeedRoom(t *testing.T) {
	for _, size := range []struct {
		w       int
		details bool
	}{{120, true}, {90, true}, {70, false}, {40, false}} {
		m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: size.w, Height: 30}, tracksMsg{tracks: demoTracks(3)})
		out := escapes.ReplaceAllString(m.View().Content, "")
		if got := strings.Contains(out, "Copy session"); got != size.details {
			t.Errorf("%d columns: details shown %v, want %v", size.w, got, size.details)
		}
		lines := strings.Split(out, "\n")
		for i, l := range lines {
			if w := len([]rune(l)); w > size.w {
				t.Fatalf("%d columns: line %d is %d wide", size.w, i, w)
			}
		}
	}
}

func TestFastTrackGivesWayToDetails(t *testing.T) {
	for _, size := range []struct {
		h         int
		fastTrack bool
	}{{50, true}, {30, false}} {
		m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: 120, Height: size.h}, tracksMsg{tracks: demoTracks(3)})
		out := escapes.ReplaceAllString(m.View().Content, "")
		if got := strings.Contains(out, "Fast Track"); got != size.fastTrack {
			t.Errorf("%d lines: Fast Track shown %v, want %v", size.h, got, size.fastTrack)
		}
		if !strings.Contains(out, "Open PR") {
			t.Errorf("%d lines: the details lost their buttons", size.h)
		}
	}
}
