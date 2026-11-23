package widget

import (
	"slices"
	"testing"

	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
)

func TestStatusBadgesAreThemeTokens(t *testing.T) {
	for _, s := range slices.Concat(track.Statuses, track.PRStatuses) {
		bg, text := BadgeTokens(s)
		if !slices.Contains(theme.All, bg) || !slices.Contains(theme.All, text) {
			t.Errorf("%s's badge %q isn't a theme state", s.ID, s.Badge)
		}
		wantBg, wantText := theme.StateInfoBg, theme.StateInfoText
		switch s {
		case track.ActionRequired, track.Exited, track.Draft:
			wantBg, wantText = theme.StateWarningBg, theme.StateWarningText
		case track.Error:
			wantBg, wantText = theme.StateDangerBg, theme.StateDangerText
		}
		if bg != wantBg || text != wantText {
			t.Errorf("%s is drawn in %s on %s, want %s on %s", s.ID, text, bg, wantText, wantBg)
		}
	}
}
