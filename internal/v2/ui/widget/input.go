// Package widget holds UI pieces more than one screen uses.
package widget

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
)

// tail cuts s from the left to fit width: the end of a path is the part
// that tells paths apart.
func tail(s string, width int) string {
	r := []rune(s)
	if width < 2 || len(r) <= width {
		return s
	}
	return "…" + string(r[len(r)-width+1:])
}

// NewInput is a text input without prompt or blinking cursor.
func NewInput(limit int, placeholder string) textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = limit
	in.Placeholder = placeholder
	s := in.Styles()
	s.Cursor.Blink = false
	in.SetStyles(s)
	return in
}

// Input draws in between brackets in bracket's colour, width cells
// on input.bg inside them. Only the focused input is drawn by
// textinput, with the theme's colours: its own styles would override
// them, and a blurred input would keep a cursor cell in the terminal's
// colours.
func Input(p style.Palette, in textinput.Model, width int, bracket theme.Token) string {
	b := lipgloss.NewStyle().Foreground(p.Color(bracket))
	bg := lipgloss.NewStyle().Background(p.Color(theme.InputBg))
	var text string
	switch {
	case in.Focused():
		s := in.Styles()
		s.Focused.Text = bg.Foreground(p.Color(theme.TextDefault))
		s.Focused.Placeholder = bg.Foreground(p.Color(theme.TextFaint))
		s.Cursor.Color = p.Color(theme.Accent)
		in.SetStyles(s)
		text = in.View()
	case in.Value() == "":
		text = bg.Foreground(p.Color(theme.TextFaint)).Render(in.Placeholder)
	default:
		text = bg.Foreground(p.Color(theme.TextMuted)).Render(tail(in.Value(), width))
	}
	text = lipgloss.NewStyle().MaxWidth(width).Render(text)
	return b.Render("[") + text + bg.Render(strings.Repeat(" ", max(0, width-lipgloss.Width(text)))) + b.Render("]")
}
