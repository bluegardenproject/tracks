package footer

import (
	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// actionsRow holds the shown track's buttons, read from its window's
// options, so it changes with the track. Without buttons, the track's
// info is centred, muted like the version; with some, such as a
// reviewer's, which last as long as the track, it sits on the left and
// the buttons centred beside it. A PR's comments are on the Platform's
// Signals. The daemon writes the buttons as a format, expanded here so
// the theme's colours fill in.
func actionsRow(p palette) string {
	info := p.fg(theme.FooterMuted) + " #{" + trackwin.InfoOption + "}#[default]"
	withButtons := "#[align=left]" + info + "#[align=centre]#{E:" + actions.ButtonsOption + "}"
	return "#{?#{" + actions.ButtonsOption + "}," + withButtons + ",#[align=centre]" + info + "}"
}

// Options are the colours the buttons and badges the daemon writes are
// drawn in: the footer's buttons, and a train's overlay header.
func Options(t theme.Theme) map[string]string {
	options := map[string]string{actions.Muted: t.Value(theme.FooterMuted)}
	for i, v := range theme.ButtonVariants {
		options[actions.VariantBg(i+1)] = t.Value(v[0])
		options[actions.VariantText(i+1)] = t.Value(v[1])
	}
	options[actions.VariantBg(actions.EndTrainVariant)] = t.Value(theme.ButtonVariant5Bg)
	options[actions.VariantText(actions.EndTrainVariant)] = t.Value(theme.ButtonVariant5Text)
	for _, kind := range actions.BadgeKinds {
		options[actions.BadgeBg(kind)] = t.Value(theme.Token("state." + kind + ".bg"))
		options[actions.BadgeText(kind)] = t.Value(theme.Token("state." + kind + ".text"))
	}
	return options
}
