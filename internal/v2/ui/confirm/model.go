// Package confirm is a popup that asks one question: a title, a few
// lines saying what happens, the action's button and Cancel.
package confirm

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// Keys are the keys the popup takes, for its hint row.
var Keys = []widget.KeyHelp{{Key: "←/→", Help: "select"}, {Key: "Enter", Help: "choose"}, {Key: "Esc", Help: "cancel"}}

// Question is what the popup asks: Action names the button that
// confirms, drawn as a danger button.
type Question struct {
	Title  string
	Text   []string
	Action string
}

// Width is the popup's width in cells for q.
func (q Question) Width() int {
	w := max(30, lipgloss.Width(q.Title)+6, lipgloss.Width(widget.Hints(lipgloss.NewStyle(), lipgloss.NewStyle(), Keys))+2)
	for _, line := range q.Text {
		w = max(w, lipgloss.Width(line)+4)
	}
	return w
}

// Height is the popup's height for q: the text, a blank line and the
// buttons in their frame, then the hint row.
func (q Question) Height() int { return len(q.Text) + 5 }

// Model is the popup. Confirmed reports the answer once it quits.
type Model struct {
	q             Question
	palette       style.Palette
	focus, hover  int // 0 is the action, 1 Cancel; hover -1 is neither
	width, height int
	confirmed     bool
}

// New returns the popup asking q, drawn in t, with the action focused.
func New(t theme.Theme, q Question) Model {
	return Model{q: q, palette: style.New(t), hover: -1}
}

// Confirmed reports whether the action was chosen.
func (m Model) Confirmed() bool { return m.confirmed }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "left", "right", "tab", "shift+tab", "h", "l":
			m.focus = 1 - m.focus
		case "enter", "space":
			return m.answer(m.focus == 0)
		case "y":
			return m.answer(true)
		case "n", "esc", "q", "ctrl+c":
			return m.answer(false)
		}
	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		if i, ok := m.buttonAt(mouse.X, mouse.Y); ok && mouse.Button == tea.MouseLeft {
			return m.answer(i == 0)
		}
	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		m.hover = -1
		if i, ok := m.buttonAt(mouse.X, mouse.Y); ok {
			m.hover = i
		}
	}
	return m, nil
}

func (m Model) answer(yes bool) (tea.Model, tea.Cmd) {
	m.confirmed = yes
	return m, tea.Quit
}

func (m Model) buttons() (string, []int, []int) {
	action := widget.NewButton(m.q.Action, widget.ButtonDanger)
	cancel := widget.NewButton("Cancel", widget.ButtonDefault)
	action.Hover = m.focus == 0 || m.hover == 0
	cancel.Hover = m.focus == 1 || m.hover == 1
	row, starts := widget.ButtonRow(m.palette, action, cancel)
	return row, starts, []int{action.Width(), cancel.Width()}
}

// buttonAt is the button drawn at cell x, y: 0 the action, 1 Cancel.
func (m Model) buttonAt(x, y int) (int, bool) {
	if y != len(m.q.Text)+2 {
		return 0, false
	}
	_, starts, widths := m.buttons()
	for i, start := range starts {
		if x >= 2+start && x < 2+start+widths[i] {
			return i, true
		}
	}
	return 0, false
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	fg := func(t theme.Token) lipgloss.Style { return lipgloss.NewStyle().Foreground(m.palette.Color(t)) }
	body := make([]string, 0, len(m.q.Text)+2)
	for _, line := range m.q.Text {
		body = append(body, fg(theme.TextMuted).Render(line))
	}
	row, _, _ := m.buttons()
	body = append(body, "", row)
	lines := widget.Frame(m.palette, m.q.Title, body, m.width, min(m.height-1, len(body)+2), theme.OverlayBorderFocus)
	lines = append(lines, " "+widget.Hints(fg(theme.TextAccent), fg(theme.TextFaint), Keys))
	return strings.Join(lines, "\n")
}
