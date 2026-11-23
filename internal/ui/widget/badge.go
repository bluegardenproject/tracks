package widget

import (
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/style"
)

// badgeTokens are the background and text tokens s's badge is drawn in.
func badgeTokens(s track.Status) (bg, text theme.Token) {
	return theme.Token("state." + s.Badge + ".bg"), theme.Token("state." + s.Badge + ".text")
}

// Badge draws s as a badge in p's colours; "" for a status without a
// label.
func Badge(p style.Palette, s track.Status) string {
	if s.Label == "" {
		return ""
	}
	bg, text := badgeTokens(s)
	return lipgloss.NewStyle().Background(p.Color(bg)).Foreground(p.Color(text)).Render(" " + s.Label + " ")
}
