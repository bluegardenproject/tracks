package widget

import (
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/style"
)

func TestBadge(t *testing.T) {
	p := style.New(theme.Default())
	if got := Badge(p, track.Status{}); got != "" {
		t.Errorf("a status without a label drew %q, want nothing", got)
	}
	got := Badge(p, track.ActionRequired)
	if !strings.Contains(got, " action required ") || lipgloss.Width(got) != len(" action required ") {
		t.Errorf("action required drew %q", got)
	}
}

func TestStatusBadgesAreThemeTokens(t *testing.T) {
	for _, s := range slices.Concat(track.Statuses, track.PRStatuses) {
		bg, text := badgeTokens(s)
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
