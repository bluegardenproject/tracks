package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
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
	if len(ended) != 0 || m.station.asking != nil {
		t.Fatalf("cancelled twice, yet ended %v (asking: %v)", ended, m.station.asking)
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
	}{{50, true}, {29, false}} {
		m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: 120, Height: size.h}, tracksMsg{tracks: demoTracks(3)})
		out := escapes.ReplaceAllString(m.View().Content, "")
		if got := strings.Contains(out, "Fast Track"); got != size.fastTrack {
			t.Errorf("%d lines: Fast Track shown %v, want %v", size.h, got, size.fastTrack)
		}
		if !strings.Contains(out, "Copy session") {
			t.Errorf("%d lines: the details lost their buttons", size.h)
		}
	}
}

func TestNameAndSlug(t *testing.T) {
	m := withEnded(Config{})
	if view := plainView(m); strings.Count(view, "│ Rate bug on checkout ") != 2 || !strings.Contains(view, "Slug      rate-bug") {
		t.Errorf("the list row and the details should show the name, the details the slug too:\n%s", view)
	}
	if view := plainView(settle(m, tea.KeyPressMsg{Code: tea.KeyDown})); !strings.Contains(view, "│ old-fix ") || !strings.Contains(view, "Slug      old-fix") {
		t.Errorf("a track without a name shows its slug:\n%s", view)
	}
}

func TestDetailsShowWhenCreated(t *testing.T) {
	m := withEnded(Config{})
	if view := plainView(m); !strings.Contains(view, "Created   2026-09-28 10:15") {
		t.Errorf("details lack when rate-bug was created:\n%s", view)
	}
	if view := plainView(settle(m, tea.KeyPressMsg{Code: tea.KeyDown})); !strings.Contains(view, "Created   unknown") {
		t.Errorf("details of a track without a creation time:\n%s", view)
	}
}

func TestDetailsShowWhereATrackStarted(t *testing.T) {
	tracks := []source.Track{
		{ID: "a", Number: 1, Name: "station-bug", Title: "Station bug", Kind: "work", Status: track.Active},
		{ID: "b", Number: 2, Name: "theme-idea", Kind: "plan", Status: track.Active, Parent: "Station bug"},
	}
	m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: tracks})
	if view := plainView(m); strings.Contains(view, "Started from") {
		t.Errorf("a track started by the user shows where it started:\n%s", view)
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if view := plainView(m); !strings.Contains(view, "Started from Station bug") {
		t.Errorf("details lack the track theme-idea was started from:\n%s", view)
	}
}

func TestLiveReloadClearsStaleConfirmationAndTracksMovedTarget(t *testing.T) {
	var ended []int
	tracks := []source.Track{
		{ID: "a", Number: 1, Name: "first", Kind: "work", Status: track.Active},
		{ID: "b", Number: 2, Name: "second", Kind: "work", Status: track.Active},
		{ID: "c", Number: 3, Name: "third", Kind: "work", Status: track.Active},
	}
	m := New(Config{Version: "test", Theme: theme.Default(), End: func(n int) error {
		ended = append(ended, n)
		return nil
	}})
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: tracks})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown}) // select "second"
	m = run(m, tea.KeyPressMsg{Code: 'e'})
	if m.station.asking == nil {
		t.Fatal("expected askEnd confirmation")
	}

	// If "second" disappears on reload, the confirmation must be cleared so
	// confirming does not end the newly selected track.
	m = update(m, tracksMsg{tracks: []source.Track{tracks[0], tracks[2]}})
	if m.station.asking != nil {
		t.Fatalf("stale confirmation stayed open after target disappeared: %+v", m.station.asking)
	}
	m = run(m, tea.KeyPressMsg{Code: 'y'})
	if len(ended) != 0 {
		t.Fatalf("y after target disappeared ended tracks %v", ended)
	}

	// If the target track moves position and changes window number, confirmation
	// follows it and ends the updated window number.
	m = update(m, tracksMsg{tracks: tracks})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp}) // move from "third" to "second"
	m = run(m, tea.KeyPressMsg{Code: 'e'})
	moved := []source.Track{
		tracks[2],
		tracks[0],
		{ID: "b", Number: 9, Name: "second", Kind: "work", Status: track.Active},
	}
	m = update(m, tracksMsg{tracks: moved})
	if m.station.asking == nil || m.station.asking.number != 9 {
		t.Fatalf("expected asking to track moved target with number 9, got %+v", m.station.asking)
	}
	m = run(m, tea.KeyPressMsg{Code: 'y'})
	if len(ended) != 1 || ended[0] != 9 {
		t.Fatalf("ended = %v, want [9]", ended)
	}

	// If the target track closes while askEnd is open, askEnd clears.
	m = update(m, tracksMsg{tracks: tracks})
	m = run(m, tea.KeyPressMsg{Code: 'e'})
	closed := []source.Track{
		tracks[0],
		{ID: "b", Number: 0, Name: "second", Kind: "work", Status: track.Done},
	}
	m = update(m, tracksMsg{tracks: closed})
	if m.station.asking != nil {
		t.Fatalf("askEnd stayed open after target closed: %+v", m.station.asking)
	}

	// askRecreate clears if the ended track becomes Closed or Archived on reload,
	// and a late checkedMsg for an open track is ignored.
	endedTrack := source.Track{ID: "b", Number: 0, Name: "second", Kind: "work", Status: track.Done}
	m = update(m, tracksMsg{tracks: []source.Track{endedTrack}})
	m = m.ask(question{id: "b", name: "second", kind: askRecreate, lines: []string{"api"}})
	if m.station.asking == nil {
		t.Fatal("expected askRecreate to open for ended track")
	}
	m = update(m, tracksMsg{tracks: []source.Track{{ID: "b", Number: 0, Name: "second", Kind: "work", Status: track.Closed}}})
	if m.station.asking != nil {
		t.Fatalf("askRecreate stayed open after track became Closed: %+v", m.station.asking)
	}
	m = update(m, tracksMsg{tracks: []source.Track{{ID: "b", Number: 2, Name: "second", Kind: "work", Status: track.Active}}})
	m = m.checked(checkedMsg{id: "b", name: "second", kind: askArchive})
	if m.station.asking != nil {
		t.Fatalf("checkedMsg opened askArchive for an open track: %+v", m.station.asking)
	}
}

func TestWideAndEscapedTrackNamesDontPanicOrLeak(t *testing.T) {
	tracks := []source.Track{
		{ID: "a", Number: 1, Name: "日本語の長いトラック名\x1b]0;title\x07\u202e", Kind: "work", Status: track.Active,
			Repos: []source.Repo{{Name: "repo\x1b[31m", Branch: "feat/日本語", Path: "/tmp/日本語のパス"}}},
	}
	m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: 42, Height: 20}, tracksMsg{tracks: tracks})
	out := m.View().Content
	if strings.Contains(out, "\x1b]0;") || strings.Contains(out, "\u202e") {
		t.Errorf("terminal escape or BiDi leaked into view: %q", out)
	}
}
