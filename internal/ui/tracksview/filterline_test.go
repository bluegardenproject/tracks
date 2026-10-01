package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

func TestFilteredLine(t *testing.T) {
	var set []track.Filter
	m := New(Config{Version: "test", Theme: theme.Default(), SetFilter: func(f track.Filter) error {
		set = append(set, f)
		return nil
	}})
	filter := track.Filter{Statuses: []string{"done"}, Started: track.Last7Days}
	tracks := []source.Track{
		{ID: "b", Name: "rate-bug", Kind: "work", Status: track.Done},
		{ID: "c", Name: "old-fix", Kind: "work", Status: track.Done},
	}
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: tracks, filter: filter})
	view := plainView(m)
	for _, want := range []string{"Filtered: done · started in the last 7 days", "x Clear filter", "rate-bug", "x clear filter"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}

	// The rows start below the Filtered line: clicking old-fix selects it.
	lines := strings.Split(view, "\n")
	for y, line := range lines {
		if x := strings.Index(line, "old-fix"); x >= 0 {
			m = settle(m, tea.MouseClickMsg{X: len([]rune(line[:x])), Y: y, Button: tea.MouseLeft})
		}
	}
	if sel, _ := m.selectedTrack(); sel.ID != "c" {
		t.Errorf("clicking old-fix selected %s", sel.ID)
	}

	m = settle(m, key('x'))
	if len(set) != 1 || set[0].On() || m.station.notice.text != "Cleared the filter." {
		t.Fatalf("x: set %+v, hint row %q", set, m.station.notice.text)
	}
	for y, line := range lines {
		if x := strings.Index(line, clearFilterLabel); x >= 0 {
			m = settle(m, tea.MouseClickMsg{X: len([]rune(line[:x])), Y: y, Button: tea.MouseLeft})
		}
	}
	if len(set) != 2 {
		t.Errorf("clicking Clear filter: set %+v", set)
	}

	m = update(m, tracksMsg{filter: track.Filter{Archived: true}})
	view = plainView(m)
	if !strings.Contains(view, "No tracks match the filter.") || strings.Contains(view, addTrackLabel) {
		t.Errorf("an empty filtered list should say so, without Add new Track:\n%s", view)
	}
	if m = settle(m, tea.KeyPressMsg{Code: tea.KeyEnter}); m.station.notice.err {
		t.Errorf("Enter on an empty filtered list: %+v", m.station.notice)
	}
}

func TestUnarchive(t *testing.T) {
	var unarchived []string
	m := New(Config{Version: "test", Theme: theme.Default(), Unarchive: func(id string) error {
		unarchived = append(unarchived, id)
		return nil
	}})
	tracks := []source.Track{{ID: "b", Name: "rate-bug", Kind: "work", Status: track.Closed, Archived: true}}
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: tracks, filter: track.Filter{Archived: true}})
	view := plainView(m)
	for _, want := range []string{" Unarchive ", "Enter unarchive", "Filtered: archived"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %q:\n%s", want, view)
		}
	}
	for _, gone := range []string{" Resume ", " Archive ", " Clean "} {
		if strings.Contains(view, gone) {
			t.Errorf("an archived track offers %q:\n%s", gone, view)
		}
	}
	if m = settle(m, key('u')); len(unarchived) != 1 || m.station.notice.text != "Unarchived rate-bug." {
		t.Errorf("u: unarchived %v, hint row %q", unarchived, m.station.notice.text)
	}
}
