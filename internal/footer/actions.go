package footer

import (
	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// actionsRow holds the shown track's buttons, read from its window's
// options, so it changes with the track. The track's info sits on the
// left, muted like the version, and the buttons centred beside it: a
// reviewer's button lasts as long as the track, so it can't take the
// info's place. The daemon writes the buttons as a format, expanded here
// so the theme's colours fill in.
func actionsRow(p palette) string {
	info := p.fg(theme.FooterMuted) + " #{" + trackwin.InfoOption + "}#[default]"
	return "#[align=left]" + info + "#[align=centre]#{E:" + actions.ButtonsOption + "}"
}

// Options are the colours the buttons the daemon writes are drawn in.
func Options(t theme.Theme) map[string]string {
	options := map[string]string{actions.Muted: t.Value(theme.FooterMuted)}
	for i, v := range theme.ButtonVariants {
		options[actions.VariantBg(i+1)] = t.Value(v[0])
		options[actions.VariantText(i+1)] = t.Value(v[1])
	}
	for v, tokens := range map[int][2]theme.Token{
		5: {theme.ButtonVariant5Bg, theme.ButtonVariant5Text},
		6: {theme.ButtonVariant6Bg, theme.ButtonVariant6Text},
	} {
		options[actions.VariantBg(v)] = t.Value(tokens[0])
		options[actions.VariantText(v)] = t.Value(tokens[1])
	}
	return options
}
