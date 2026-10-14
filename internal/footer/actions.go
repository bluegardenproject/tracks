package footer

import (
	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// actionsRow holds the shown track's buttons, read from its window's
// options, so it changes with the track. It sits centred above the track
// list. The daemon writes the buttons as a format, expanded here so the
// theme's colours fill in. A track without any shows its info, muted
// like the version.
func actionsRow(p palette) string {
	info := p.fg(theme.FooterMuted) + "#{" + trackwin.InfoOption + "}#[default]"
	return "#[align=centre]#{?" + actions.ButtonsOption + ",#{E:" + actions.ButtonsOption + "}," + info + "}"
}

// Options are the colours the buttons the daemon writes are drawn in.
func Options(t theme.Theme) map[string]string {
	options := map[string]string{actions.Muted: t.Value(theme.FooterMuted)}
	for i, v := range theme.ButtonVariants {
		options[actions.VariantBg(i+1)] = t.Value(v[0])
		options[actions.VariantText(i+1)] = t.Value(v[1])
	}
	return options
}
