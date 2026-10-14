package footer

import (
	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/theme"
)

// actionsRow holds the shown track's actions, read from its window's
// options, so it changes with the track. It sits centred above the track
// list. A button is a user range named after its action: clicking it runs
// the action. A track without any leaves the row empty.
func actionsRow(p palette) string {
	return "#[align=centre]" + commentsButton(p)
}

// commentsButton offers to send the open review comments to the agent,
// and says so, muted, once they're sent.
func commentsButton(p palette) string {
	open := "#{" + actions.CommentsOption + "}"
	label := " " + open + " open #{?#{==:" + open + ",1},comment,comments} · "
	button := "#[range=user|" + actions.PRComments + "]" + p.bg(theme.ButtonBgAccent) + p.fg(theme.ButtonTextAccent) +
		label + "solve now #[norange default]"
	sent := p.fg(theme.FooterMuted) + label + "sent to the agent #[default]"
	return "#{?" + actions.CommentsOption + ",#{?" + actions.SentOption + "," + sent + "," + button + "},}"
}
