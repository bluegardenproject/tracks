package tracksview

import (
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// statusBadges are t's track status and, when it has them, its PR
// status and its errors' status, as badges with a gap in fill between
// them.
func (m Model) statusBadges(t source.Track, fill lipgloss.Style) string {
	out := widget.Badge(m.palette, t.Status)
	for _, s := range []track.Status{t.PRStatus, t.FailureStatus} {
		if s.Label != "" {
			out += fill.Render(" ") + widget.Badge(m.palette, s)
		}
	}
	return out
}

// statusText is statusBadges as plain text, as wide.
func statusText(t source.Track) string {
	out := " " + t.Status.Label + " "
	for _, s := range []track.Status{t.PRStatus, t.FailureStatus} {
		if s.Label != "" {
			out += "  " + s.Label + " "
		}
	}
	return out
}
