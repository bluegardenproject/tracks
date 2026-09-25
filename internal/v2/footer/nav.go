package footer

import "github.com/bluegardenproject/tracks/internal/v2/theme"

// nav is the navigation row: the Tracks window pinned on the left, the
// track windows centred. When the tracks don't fit, tmux scrolls the
// list to keep the active one visible and marks the cut edges.
func nav(p palette) string {
	active := p.bg(theme.FooterActiveBg) + p.fg(theme.FooterActiveText) + "#[bold]"
	isTracks := "#{==:#{window_index},0}"

	tracks := "#[range=window|0]#{?" + isTracks + "," + active + ",} Tracks #[norange default]"

	slot := " #{window_index} #{window_name} "
	other := "#[range=window|#{window_index}]" + slot + "#[norange list=on default]"
	current := "#[range=window|#{window_index} list=focus]" + active + slot + "#[norange list=on default]"
	list := "#{W:#{?" + isTracks + ",," + other + "},#{?" + isTracks + ",," + current + "}}"

	return "#[align=left]" + tracks +
		"#[list=on align=centre]#[list=left-marker]‹#[list=right-marker]›#[list=on]" + list +
		"#[nolist]"
}
