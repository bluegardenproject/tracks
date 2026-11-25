package tracksview

import (
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// statuses are t's track status, its PR status and its errors' status;
// the last two have no Label when it has none.
func statuses(t source.Track) []track.Status {
	return []track.Status{t.Status, t.PRStatus, t.FailureStatus}
}

// statusBadges are t's statuses as badges, those it has, with a gap in
// fill between them.
func (m Model) statusBadges(t source.Track, fill lipgloss.Style) string {
	out := ""
	for _, s := range statuses(t) {
		if s.Label == "" {
			continue
		}
		if out != "" {
			out += fill.Render(" ")
		}
		out += widget.Badge(m.palette, s)
	}
	return out
}

// badgeText is s's badge as plain text, as wide; "" when s has no label.
func badgeText(s track.Status) string {
	if s.Label == "" {
		return ""
	}
	return " " + s.Label + " "
}
