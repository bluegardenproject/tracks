package tracksview

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

func demoTracks(n int) []source.Track {
	tracks := make([]source.Track, n)
	for i := range tracks {
		tracks[i] = source.Track{Number: i + 1, Name: fmt.Sprintf("track-%02d", i+1), Kind: "feature", Status: source.Running,
			Repos: []source.Repo{{Name: "shop", Branch: "main", Path: "/tmp/shop"}}, Session: "s-1"}
	}
	return tracks
}

// withTracks returns a Station showing tracks, recording what it opens.
func withTracks(tracks []source.Track, w, h int, opened *[]int) Model {
	m := New(Config{Version: "test", Theme: theme.Default(), Open: func(n int) error {
		*opened = append(*opened, n)
		return nil
	}})
	return update(m, tea.WindowSizeMsg{Width: w, Height: h}, tracksMsg{tracks: tracks})
}

func run(m Model, msg tea.Msg) Model {
	next, cmd := m.Update(msg)
	m = next.(Model)
	if cmd != nil {
		if out := cmd(); out != nil {
			m = update(m, out)
		}
	}
	return m
}

func TestStationKeysSelectAndOpen(t *testing.T) {
	var opened []int
	m := withTracks(demoTracks(3), 120, 40, &opened)
	for _, key := range []tea.KeyPressMsg{{Code: 'j'}, {Code: tea.KeyDown}, {Code: tea.KeyDown}, {Code: 'k'}} {
		m = update(m, key)
	}
	if m.station.selected != 1 {
		t.Fatalf("selected %d, want 1 (down, down, down past the end, up)", m.station.selected)
	}
	m = run(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(opened) != 1 || opened[0] != 2 {
		t.Errorf("Enter opened %v, want track 2", opened)
	}
}

func TestStationClicks(t *testing.T) {
	var opened []int
	m := withTracks(demoTracks(3), 120, 40, &opened)
	y := m.stationTop() + 2
	m = run(m, tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
	if m.station.selected != 2 || len(opened) != 0 {
		t.Fatalf("one click: selected %d, opened %v; want row 2 selected, nothing opened", m.station.selected, opened)
	}
	m = run(m, tea.MouseClickMsg{X: 10, Y: y, Button: tea.MouseLeft})
	if len(opened) != 1 || opened[0] != 3 {
		t.Errorf("double click opened %v, want track 3", opened)
	}
	if m = run(m, tea.MouseClickMsg{X: 10, Y: m.stationTop() + 3, Button: tea.MouseLeft}); m.station.selected != 2 {
		t.Errorf("a click below the last track changed the selection to %d", m.station.selected)
	}
}

func TestStationScrollsToSelection(t *testing.T) {
	var opened []int
	m := withTracks(demoTracks(30), 100, 20, &opened)
	for range 29 {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	out := m.View().Content
	if !strings.Contains(out, "track-30") || strings.Contains(out, "track-01") {
		t.Error("with the last track selected, the list should show it and have scrolled past the first")
	}
	for range 29 {
		m = update(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	}
	if m.station.selected != 0 || !strings.Contains(m.View().Content, "track-01") {
		t.Error("wheeling up should bring back the first track")
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 20 {
		t.Errorf("view has %d lines, want 20", len(lines))
	}
}

func TestStationEmptyAndFailing(t *testing.T) {
	m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: 100, Height: 30}, tracksMsg{})
	if !strings.Contains(m.View().Content, "No tracks yet.") {
		t.Error("no tracks: missing the empty message")
	}
	m = update(m, tracksMsg{err: errors.New("server gone")})
	if !strings.Contains(m.View().Content, "server gone") {
		t.Error("failing source: missing the error")
	}
}

func TestStationHover(t *testing.T) {
	var opened []int
	m := withTracks(demoTracks(3), 120, 40, &opened)
	if m = update(m, tea.MouseMotionMsg{X: 10, Y: m.stationTop() + 1}); m.station.hover != 1 {
		t.Errorf("hovering the second row: hover %d, want 1", m.station.hover)
	}
	if m = update(m, tea.MouseMotionMsg{X: 10, Y: 0}); m.station.hover != -1 {
		t.Errorf("leaving the rows: hover %d, want none", m.station.hover)
	}
}
