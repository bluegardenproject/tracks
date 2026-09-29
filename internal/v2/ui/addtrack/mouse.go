package addtrack

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// fit sizes the fields to the popup, gives the prompt the room left and
// scrolls the focus into view.
func (m Model) fit() Model {
	inner := m.width - 4
	if inner < 4 {
		return m
	}
	for _, in := range []*textinput.Model{&m.target, &m.document, &m.name} {
		in.SetWidth(max(1, inner-3))
	}
	m.prompt.SetWidth(max(1, inner-2))
	m.prompt.SetHeight(minPrompt)
	lines, _ := m.body(inner)
	m.prompt.SetHeight(max(minPrompt, min(maxPrompt, minPrompt+m.rows()-len(lines))))
	return m.follow()
}

// follow scrolls so the focused field shows, or at least its line.
func (m Model) follow() Model {
	lines, hits := m.body(max(1, m.width-4))
	rows := m.rows()
	for _, h := range hits {
		if h.ctl != m.focus || (h.index != m.item && h.ctl != ctlType) || (h.ctl == ctlType && h.index != int(m.kind)) {
			continue
		}
		top, bottom := h.top, h.bottom
		if bottom-top > rows {
			top, bottom = h.y, h.y+min(h.h, rows)
		}
		if top < m.offset {
			m.offset = top
		}
		if bottom > m.offset+rows {
			m.offset = bottom - rows
		}
		break
	}
	m.offset = max(0, min(m.offset, len(lines)-rows))
	return m
}

// at is the control drawn at screen cell x, y.
func (m Model) at(x, y int) hit {
	if y < 1 || y > m.rows() {
		return noHit
	}
	bx, by := x-2, y-1+m.offset
	_, hits := m.body(m.width - 4)
	for _, h := range hits {
		if bx >= h.x && bx < h.x+h.w && by >= h.y && by < h.y+h.h {
			return h
		}
	}
	return noHit
}

func (m Model) click(mouse tea.Mouse) (Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	switch {
	case m.discard != nil:
		if i, ok := m.discardAt(mouse.X, mouse.Y); ok {
			return m.answer(i)
		}
		return m, nil
	case m.picker != nil:
		x, y, w, h := m.pickerBox()
		if mouse.X < x || mouse.X >= x+w || mouse.Y < y || mouse.Y >= y+h {
			return m.pickerDone(widget.PickerClosed), nil
		}
		p := *m.picker
		m.picker = &p
		return m.pickerDone(p.Click(mouse.X-x, mouse.Y-y, w)), nil
	}
	m.notice = ""
	h := m.at(mouse.X, mouse.Y)
	if h.ctl < 0 {
		return m, nil
	}
	switch h.ctl {
	case ctlCreate, ctlCancel:
		return m.press(h.ctl)
	}
	if h.ctl != m.focus {
		m = m.setFocus(h.ctl)
	}
	m.item = h.index
	switch h.ctl {
	case ctlType:
		m = m.setKind(Kind(h.index))
	case ctlRepos, ctlRepo, ctlSections:
		m = m.toggle(h.ctl, h.index)
	case ctlTerminal:
		m.terminal = !m.terminal
	case ctlCandor:
		m = m.openCandor()
	}
	return m, nil
}

func (m Model) motion(mouse tea.Mouse) Model {
	switch {
	case m.discard != nil:
		q := *m.discard
		q.hover = -1
		if i, ok := m.discardAt(mouse.X, mouse.Y); ok {
			q.hover = i
		}
		m.discard = &q
	case m.picker != nil:
		x, y, w, _ := m.pickerBox()
		p := *m.picker
		p.Hover(mouse.X-x, mouse.Y-y, w)
		m.picker = &p
	default:
		m.hover = m.at(mouse.X, mouse.Y)
	}
	return m
}

func (m Model) wheel(mouse tea.Mouse) Model {
	step := 3
	if mouse.Button == tea.MouseWheelUp {
		step = -3
	}
	if m.picker != nil {
		p := *m.picker
		p.Wheel(step / 3)
		m.picker = &p
		return m
	}
	lines, _ := m.body(max(1, m.width-4))
	m.offset = max(0, min(m.offset+step, len(lines)-m.rows()))
	return m
}

// pickerBox is where the candor picker is drawn: centred.
func (m Model) pickerBox() (x, y, w, h int) {
	w, h = m.picker.Size(m.width, m.height)
	return (m.width - w) / 2, (m.height - h) / 2, w, h
}

const discardText = "What you entered is lost."

// discardBox is the question over the form, centred, and where it goes.
func (m Model) discardBox() (x, y int, box []string) {
	row, _ := m.discardButtons()
	w := min(m.width, max(len(discardText), 30)+4)
	box = widget.Frame(m.palette, "Discard this track?", []string{m.fg(theme.TextMuted).Render(discardText), "", row}, w, 5, theme.OverlayBorderFocus)
	return (m.width - w) / 2, (m.height - 5) / 2, box
}

func (m Model) discardButtons() (string, []int) {
	q := m.discard
	discard := widget.NewButton("Discard", widget.ButtonDanger)
	keep := widget.NewButton("Keep editing", widget.ButtonDefault)
	discard.Hover = q.focus == 0 || q.hover == 0
	keep.Hover = q.focus == 1 || q.hover == 1
	return widget.ButtonRow(m.palette, discard, keep)
}

// discardAt is the question's button at screen cell x, y: 0 discards.
func (m Model) discardAt(x, y int) (int, bool) {
	bx, by, _ := m.discardBox()
	_, starts := m.discardButtons()
	if y != by+3 {
		return 0, false
	}
	widths := []int{len("Discard") + 2, len("Keep editing") + 2}
	for i, start := range starts {
		if x >= bx+2+start && x < bx+2+start+widths[i] {
			return i, true
		}
	}
	return 0, false
}
