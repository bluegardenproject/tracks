package widget

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
)

// ButtonKind picks a button's colours.
type ButtonKind int

const (
	ButtonDefault ButtonKind = iota
	ButtonAccent             // the one to press
	ButtonDanger             // destroys something
)

var buttonTokens = map[ButtonKind][3]theme.Token{
	ButtonDefault: {theme.ButtonBgDefault, theme.ButtonBgHover, theme.ButtonTextDefault},
	ButtonAccent:  {theme.ButtonBgAccent, theme.ButtonBgAccentHover, theme.ButtonTextAccent},
	ButtonDanger:  {theme.ButtonBgDanger, theme.ButtonBgDangerHover, theme.ButtonTextDanger},
}

// ButtonGap is the space between buttons in a row.
const ButtonGap = 2

// Button is a label on a coloured block, one cell of padding each side.
type Button struct {
	Label    string
	Kind     ButtonKind
	Hover    bool // under the mouse
	Disabled bool // drawn faint, as the default kind
	// Hot is the index of the label's underlined letter, its key; -1
	// for none, as NewButton sets it.
	Hot int
}

// NewButton is a kind button showing label, without a key.
func NewButton(label string, kind ButtonKind) Button {
	return Button{Label: label, Kind: kind, Hot: -1}
}

// Width is the button's width in cells.
func (b Button) Width() int { return lipgloss.Width(b.Label) + 2 }

// View draws the button in p's colours.
func (b Button) View(p style.Palette) string {
	kind := b.Kind
	if b.Disabled {
		kind = ButtonDefault
	}
	t := buttonTokens[kind]
	bg := t[0]
	if b.Hover && !b.Disabled {
		bg = t[1]
	}
	s := lipgloss.NewStyle().Background(p.Color(bg)).Foreground(p.Color(t[2])).Bold(kind != ButtonDefault)
	if b.Disabled {
		return s.Foreground(p.Color(theme.TextFaint)).Render(" " + b.Label + " ")
	}
	if b.Hot < 0 || b.Hot >= len(b.Label) {
		return s.Render(" " + b.Label + " ")
	}
	return s.Render(" "+b.Label[:b.Hot]) + s.Underline(true).Render(b.Label[b.Hot:b.Hot+1]) + s.Render(b.Label[b.Hot+1:]+" ")
}

// ButtonRow draws buttons ButtonGap apart and returns where each
// starts, for clicks.
func ButtonRow(p style.Palette, buttons ...Button) (string, []int) {
	var row strings.Builder
	starts := make([]int, len(buttons))
	x := 0
	for i, b := range buttons {
		if i > 0 {
			row.WriteString(strings.Repeat(" ", ButtonGap))
			x += ButtonGap
		}
		starts[i] = x
		row.WriteString(b.View(p))
		x += b.Width()
	}
	return row.String(), starts
}
