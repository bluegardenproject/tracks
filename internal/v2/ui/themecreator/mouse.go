package themecreator

import (
	tea "charm.land/bubbletea/v2"
)

const wheelStep = 3

// click focuses what was clicked: a value, placing the cursor where it
// was clicked, or a button, which it also presses.
func (m Model) click(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if mouse.Y == m.buttonsY() {
		x := 1
		for b := range buttonCount {
			if mouse.X >= x && mouse.X < x+buttonWidth(b) {
				focus := m.focusField(len(m.fields) + b)
				var press tea.Cmd
				m, press = m.press(b)
				return m, tea.Batch(focus, press)
			}
			x += buttonWidth(b) + buttonGap
		}
		return m, nil
	}

	token, ok := m.tokenAt(mouse.Y)
	if !ok {
		return m, nil
	}
	i, inputX := 2*token, darkInputX
	if mouse.X >= lightColumnX {
		i, inputX = i+1, lightInputX
	}
	cmd := m.focusField(i)
	if pos := mouse.X - inputX - 1; pos >= 0 && pos < inputWidth-1 {
		m.fields[i].input.SetCursor(pos)
	}
	return m, cmd
}

// tokenAt returns the token shown on screen line y.
func (m Model) tokenAt(y int) (int, bool) {
	if y < listTop || y >= listTop+m.listHeight() {
		return 0, false
	}
	line := m.offset + y - listTop
	_, tokenLine := layout()
	for i, l := range tokenLine {
		if l == line {
			return i, true
		}
	}
	return 0, false
}

// wheel scrolls the list without moving focus.
func (m Model) wheel(msg tea.MouseWheelMsg) Model {
	lines, _ := layout()
	switch msg.Mouse().Button {
	case tea.MouseWheelUp:
		m.offset = max(0, m.offset-wheelStep)
	case tea.MouseWheelDown:
		m.offset = min(max(0, lines-m.listHeight()), m.offset+wheelStep)
	}
	return m
}
