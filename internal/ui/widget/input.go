// Package widget holds UI pieces more than one screen uses.
package widget

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/bluegardenproject/tracks/internal/termtext"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/style"
)

// readClipboard reads the local system clipboard; overridden in tests.
var readClipboard = clipboard.ReadAll

// ReadClipboard reads the system clipboard into a ClipboardMsg, falling
// back to OSC 52 when the local clipboard isn't available.
func ReadClipboard() tea.Msg {
	if s, err := readClipboard(); err == nil {
		return tea.ClipboardMsg{Content: s}
	}
	return tea.ReadClipboard()
}

// UpdateInput updates in with msg, routing its Paste shortcut through
// ReadClipboard so Ctrl+V uses the local clipboard with an OSC 52 fallback
// and arrives as a ClipboardMsg.
func UpdateInput(in textinput.Model, msg tea.Msg) (textinput.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && key.Matches(k, in.KeyMap.Paste) {
		return in, ReadClipboard
	}
	return in.Update(msg)
}

// tail cuts s from the left to fit width: the end of a path is the part
// that tells paths apart.
func tail(s string, width int) string {
	return CutLeft(termtext.SingleLine(s), width)
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
	return b.Render("[") + InputText(p, in, width) + b.Render("]")
}

// InputText draws in as Input does, without the brackets: width cells
// on input.bg.
func InputText(p style.Palette, in textinput.Model, width int) string {
	bg := lipgloss.NewStyle().Background(p.Color(theme.InputBg))
	var text string
	switch {
	case in.Focused():
		s := in.Styles()
		s.Focused.Text = bg.Foreground(p.Color(theme.TextDefault))
		s.Focused.Placeholder = bg.Foreground(p.Color(theme.TextFaint))
		s.Cursor.Color = p.Color(theme.TextAccent)
		in.SetStyles(s)
		text = in.View()
	case in.Value() == "":
		text = bg.Foreground(p.Color(theme.TextFaint)).Render(in.Placeholder)
	default:
		text = bg.Foreground(p.Color(theme.TextMuted)).Render(tail(in.Value(), width))
	}
	text = lipgloss.NewStyle().MaxWidth(width).Render(text)
	return text + bg.Render(strings.Repeat(" ", max(0, width-lipgloss.Width(text))))
}
