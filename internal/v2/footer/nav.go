package footer

import (
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
)

// nav is the navigation row: the Tracks window pinned on the left, the
// track windows centred. When the tracks don't fit, tmux scrolls the
// list to keep the active one visible and marks the cut edges.
func nav(p palette) string {
	active := p.bg(theme.FooterActiveBg) + p.fg(theme.FooterActiveText) + "#[bold]"
	isTracks := "#{==:#{window_index},0}"

	tracks := "#[range=window|0]#{?" + isTracks + "," + active + ",} Tracks #[norange default]"

	// slot is a track's window; one that needs the user gets a mark,
	// after which the slot's colour comes back with restore.
	slot := func(restore string) string {
		mark := "#{?" + trackwin.AttentionOption + "," + p.fg(theme.StateWarningText) + "● " + restore + ",}"
		return " #{window_index} " + mark + "#{window_name} "
	}
	other := "#[range=window|#{window_index}]" + slot("#[fg=default]") + "#[norange list=on default]"
	current := "#[range=window|#{window_index} list=focus]" + active + slot(p.fg(theme.FooterActiveText)) + "#[norange list=on default]"
	list := "#{W:#{?" + isTracks + ",," + other + "},#{?" + isTracks + ",," + current + "}}"

	return "#[align=left]" + tracks +
		"#[list=on align=centre]#[list=left-marker]‹#[list=right-marker]›#[list=on]" + list +
		"#[nolist]"
}
