package tracksview

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

func TestStatusTextIsAsWideAsTheBadges(t *testing.T) {
	m := New(Config{Version: "test", Theme: theme.Default()})
	for _, tr := range []source.Track{
		{Status: track.Active},
		{Status: track.ActionRequired, PRStatus: track.PRsOpen},
		{Status: track.Done, PRStatus: track.PRsMerged},
	} {
		if got, want := lipgloss.Width(m.statusBadges(tr, lipgloss.NewStyle())), lipgloss.Width(statusText(tr)); got != want {
			t.Errorf("%s: badges %d wide, text %d", statusText(tr), got, want)
		}
	}
}
