package tracksview

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

func TestBadgeTextIsAsWideAsTheBadge(t *testing.T) {
	m := New(Config{Version: "test", Theme: theme.Default()})
	for _, s := range []track.Status{track.Active, track.ActionRequired, track.PRsOpen, track.PRsMerged} {
		if got, want := lipgloss.Width(widget.Badge(m.palette, s)), lipgloss.Width(badgeText(s)); got != want {
			t.Errorf("%s: badge %d wide, text %d", s.Label, got, want)
		}
	}
	if badgeText(track.Status{}) != "" {
		t.Error("no status has a text")
	}
}
