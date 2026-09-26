package themecreator

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

const wheelStep = 3

// click acts on what was clicked, x and y relative to the pane: a
// value, placing the cursor where it was clicked; a button, which it
// presses.
func (m Model) click(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if mouse.Y == m.buttonsY() {
		return m.clickBottom(mouse.X)
	}
	if mouse.Y < listTop || mouse.Y >= listTop+m.listHeight() {
		return m, nil
	}
	token, ok := m.tokenAt(mouse.Y)
	if m.mode != modeEdit || !ok {
		return m, nil
	}
	cmd := m.setFocus(token)
	if pos := mouse.X - inputX - 1; pos >= 0 && pos < inputWidth-1 {
		m.fields[token].input.SetCursor(pos)
	}
	return m, cmd
}

func (m Model) clickBottom(x int) (Model, tea.Cmd) {
	if m.mode == modeName && x >= len(namePrompt) && x < len(namePrompt)+nameInputWidth+2 {
		return m, m.setFocus(0)
	}
	i := m.buttonAt(x, m.buttonsY())
	if i < 0 {
		return m, nil
	}
	focus := m.setFocus(m.firstButton() + i)
	var press tea.Cmd
	m, press = m.press(m.buttons()[i])
	return m, tea.Batch(focus, press)
}

// buttonAt returns the index of the button at pane cell x, y, -1 for
// none.
func (m Model) buttonAt(x, y int) int {
	if y != m.buttonsY() {
		return -1
	}
	start := m.buttonsX()
	for i, b := range m.buttons() {
		if x >= start && x < start+buttonWidth(b) {
			return i
		}
		start += buttonWidth(b) + widget.ButtonGap
	}
	return -1
}

// tokenAt returns the token shown on pane line y.
func (m Model) tokenAt(y int) (int, bool) {
	line := m.offset + y - listTop
	_, tokenLine := layout()
	for i, l := range tokenLine {
		if l == line {
			return i, true
		}
	}
	return 0, false
}

// sampleAt returns the example button at pane cell x, y, -1 for none.
func (m Model) sampleAt(x, y int) int {
	if m.mode != modeEdit {
		return -1
	}
	_, tokenLine := layout()
	heading := tokenLine[slices.Index(theme.All, theme.ButtonBgDefault)] - 1
	if y != listTop+heading-m.offset || heading < m.offset {
		return -1
	}
	_, starts := widget.ButtonRow(m.palette(), sampleButtons(-1)...)
	for i, b := range sampleButtons(-1) {
		if x >= exampleX+starts[i] && x < exampleX+starts[i]+b.Width() {
			return i
		}
	}
	return -1
}

// wheel scrolls the list without moving focus.
func (m Model) wheel(msg tea.MouseWheelMsg) Model {
	step := wheelStep
	if msg.Mouse().Button == tea.MouseWheelUp {
		step = -step
	}
	lines, _ := layout()
	m.offset = max(0, min(max(0, lines-m.listHeight()), m.offset+step))
	return m
}
