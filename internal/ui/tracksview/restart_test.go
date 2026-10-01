package tracksview

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

// withExits is a track whose agent failed, one whose agent exited, and
// one whose agent runs; the first is selected.
func withExits(c Config) Model {
	c.Version, c.Theme = "test", theme.Default()
	list := []source.Track{
		{ID: "a", Number: 1, Name: "crashed", Kind: "work", Status: track.Error},
		{ID: "b", Number: 2, Name: "quit", Kind: "plan", Status: track.Exited},
		{ID: "c", Number: 3, Name: "running", Kind: "work", Status: track.Active},
	}
	return update(New(c), tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: list})
}

func TestRestartButton(t *testing.T) {
	var restarted []string
	m := withExits(Config{Restart: func(id string, progress func(string)) error {
		restarted = append(restarted, id)
		progress("Starting Claude Code…")
		return nil
	}})
	if view := plainView(m); !strings.Contains(view, " Restart ") || !strings.Contains(view, "error") {
		t.Fatalf("a failed track has no Restart or error status:\n%s", view)
	}

	next, cmd := m.Update(key('r'))
	m = next.(Model)
	notices := []string{m.station.notice.text}
	for cmd != nil {
		next, cmd = m.Update(cmd())
		m = next.(Model)
		notices = append(notices, m.station.notice.text)
	}
	if want := []string{"Restarting crashed…", "Starting Claude Code…", "Restarted crashed."}; !slices.Equal(notices, want) {
		t.Errorf("hint row %q, want %q", notices, want)
	}

	m = settle(m, tea.KeyPressMsg{Code: tea.KeyDown})
	view := plainView(m)
	if !strings.Contains(view, "agent exited") || !strings.Contains(view, " Restart ") || !strings.Contains(view, " Promote ") {
		t.Errorf("a Plan track whose agent exited should offer Restart and Promote:\n%s", view)
	}
	m = settle(settle(m, key('r')), tea.KeyPressMsg{Code: tea.KeyDown})
	if view := plainView(m); strings.Contains(view, " Restart ") {
		t.Errorf("a running agent offers Restart:\n%s", view)
	}
	m = settle(m, key('r'))
	if !slices.Equal(restarted, []string{"a", "b"}) {
		t.Errorf("restarted %v; a running agent can't be", restarted)
	}
}
