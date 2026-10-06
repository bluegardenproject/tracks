package tracksview

import (
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

// badgeTokens are the background and text tokens s's badge is drawn in.
func badgeTokens(s track.Status) (bg, text theme.Token) {
	return theme.Token("state." + s.Badge + ".bg"), theme.Token("state." + s.Badge + ".text")
}

// badge draws s as a badge; "" for a status without a label.
func (m Model) badge(s track.Status) string {
	if s.Label == "" {
		return ""
	}
	bg, text := badgeTokens(s)
	return lipgloss.NewStyle().Background(m.palette.Color(bg)).Foreground(m.palette.Color(text)).Render(" " + s.Label + " ")
}

// statusBadges are t's track status and, when it has them, its PR
// status and its errors' status, as badges with a gap in fill between
// them.
func (m Model) statusBadges(t source.Track, fill lipgloss.Style) string {
	out := m.badge(t.Status)
	for _, s := range []track.Status{t.PRStatus, t.FailureStatus} {
		if s.Label != "" {
			out += fill.Render(" ") + m.badge(s)
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
