// Package footer designs the footer, tmux's status rows at the bottom
// of every window, as tmux format strings coloured from theme tokens.
package footer

import (
	"strings"

	"github.com/bluegardenproject/tracks/internal/theme"
)

// Rows returns the footer rows, top to bottom. systemCommand prints the
// system data; tmux reruns it every status-interval. Without buttons, as
// on a tmux that can't click them, the top row stays empty.
func Rows(t theme.Theme, version, systemCommand string, buttons bool) []string {
	p := palette{t}
	top := ""
	if buttons {
		top = actionsRow(p)
	}
	return []string{
		top,
		nav(p),
		"",
		"#[align=left]" + p.fg(theme.FooterMuted) + " Tracks " + escape(version) + "#[default]" +
			"#[align=right]#(" + systemCommand + ") ",
	}
}

// escape keeps tmux from reading text as a format.
func escape(s string) string { return strings.ReplaceAll(s, "#", "##") }

type palette struct {
	theme theme.Theme
}

// fg and bg return tmux styles for token.
func (p palette) fg(token theme.Token) string { return "#[fg=" + p.theme.Value(token) + "]" }

func (p palette) bg(token theme.Token) string { return "#[bg=" + p.theme.Value(token) + "]" }
