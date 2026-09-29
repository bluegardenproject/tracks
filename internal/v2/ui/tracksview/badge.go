package tracksview

import (
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
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

// statusBadges are t's track status and, when it has PRs, its PR
// status, as badges with a gap in fill between them.
func (m Model) statusBadges(t source.Track, fill lipgloss.Style) string {
	if t.PRStatus.Label == "" {
		return m.badge(t.Status)
	}
	return m.badge(t.Status) + fill.Render(" ") + m.badge(t.PRStatus)
}

// statusText is statusBadges as plain text, as wide.
func statusText(t source.Track) string {
	if t.PRStatus.Label == "" {
		return " " + t.Status.Label + " "
	}
	return " " + t.Status.Label + "   " + t.PRStatus.Label + " "
}
