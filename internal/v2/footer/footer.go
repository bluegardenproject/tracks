// Package footer designs the footer, tmux's status rows at the bottom
// of every window, as tmux format strings coloured from theme tokens.
package footer

import (
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// Rows returns the footer rows, top to bottom. systemCommand prints the
// system data; tmux reruns it every status-interval.
func Rows(t theme.Theme, version, systemCommand string) []string {
	p := palette{t}
	return []string{
		nav(p),
		"",
		"", // reserved for later use
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
