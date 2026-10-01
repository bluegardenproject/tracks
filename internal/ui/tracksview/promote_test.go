package tracksview

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

// withPlans is an open Plan track, an ended Ask track, an Ask track
// without repos and a Work track; the first is selected.
func withPlans(c Config) Model {
	c.Version, c.Theme = "test", theme.Default()
	web := []source.Repo{{Name: "web", Path: "/src/web"}}
	list := []source.Track{
		{ID: "a", Number: 1, Name: "cache-plan", Kind: "plan", Status: track.Active, Repos: web},
		{ID: "b", Name: "why-slow", Kind: "ask", Status: track.Done, Repos: web},
		{ID: "c", Number: 2, Name: "loose-ask", Kind: "ask", Status: track.Active},
		{ID: "d", Number: 3, Name: "fix-it", Kind: "work", Status: track.Active, Repos: web},
	}
	return update(New(c), tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: list})
}

func TestPromoteButton(t *testing.T) {
	var promoted []string
	m := withPlans(Config{Promote: func(id string, progress func(string)) error {
		promoted = append(promoted, id)
		progress("Creating worktree for web…")
		return nil
	}})
	if view := plainView(m); !strings.Contains(view, " Promote ") {
		t.Fatalf("an open Plan track has no Promote:\n%s", view)
	}

	next, cmd := m.Update(key('m'))
	m = next.(Model)
	notices := []string{m.station.notice.text}
	for cmd != nil {
		next, cmd = m.Update(cmd())
		m = next.(Model)
		notices = append(notices, m.station.notice.text)
	}
	want := []string{"Promoting cache-plan…", "Creating worktree for web…", "Promoted cache-plan: it has its own worktree now."}
	if !slices.Equal(notices, want) {
		t.Errorf("hint row %q, want %q", notices, want)
	}

	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if view := plainView(m); !strings.Contains(view, " Resume ") || !strings.Contains(view, " Promote ") {
		t.Errorf("an ended Ask track has no Promote:\n%s", view)
	}
	m = settle(settle(m, key('m')), tea.KeyPressMsg{Code: tea.KeyDown})
	m = settle(settle(m, key('m')), tea.KeyPressMsg{Code: tea.KeyDown})
	if view := plainView(m); strings.Contains(view, " Promote ") {
		t.Errorf("a Work track offers Promote:\n%s", view)
	}
	m = settle(m, key('m'))
	if !slices.Equal(promoted, []string{"a", "b"}) {
		t.Errorf("promoted %v; a track without repos or a Work track can't be", promoted)
	}
}

func TestPromoteFails(t *testing.T) {
	m := withPlans(Config{Promote: func(string, func(string)) error {
		return tracks.Problem("Add Cursor on the Engines tab to promote this track.")
	}})
	if m = settle(m, key('m')); m.station.notice != (notice{text: "Add Cursor on the Engines tab to promote this track.", err: true}) {
		t.Errorf("hint row %+v", m.station.notice)
	}
}
