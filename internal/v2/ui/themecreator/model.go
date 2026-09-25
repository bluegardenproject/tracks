// Package themecreator is a screen for editing a theme: every token
// with a preview and its dark and light values. The host decides what
// applying means; this package only hands the edited theme over.
package themecreator

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
)

// ApplyMsg asks the host to apply Theme. The host answers with
// ResultMsg.
type ApplyMsg struct{ Theme theme.Theme }

// ResultMsg tells the creator whether applying worked.
type ResultMsg struct{ Err error }

// CloseMsg means the user is done (Cancel or Esc).
type CloseMsg struct{}

const (
	buttonApply = iota
	buttonCancel
	buttonCopy
	buttonCount
)

var buttonLabels = [buttonCount]string{"Apply", "Cancel", "Copy all"}

// field is one value: a token's dark or light colour.
type field struct {
	token theme.Token
	dark  bool
	input textinput.Model
}

// Model is the creator. Fields come in pairs, dark then light, in the
// order of theme.All; the buttons follow them in the focus order.
type Model struct {
	base          theme.Theme // the applied theme
	dark          bool        // the terminal's background, for the creator's own colours
	fields        []field
	focus         int
	offset        int // first visible line of the token list
	width, height int
	status        string
	statusErr     bool
}

// New returns a creator editing t.
func New(t theme.Theme, dark bool) Model {
	m := Model{base: t, dark: dark}
	for _, token := range theme.All {
		v := t.Value(token)
		m.fields = append(m.fields, newField(token, true, v.Dark), newField(token, false, v.Light))
	}
	m.focusField(0)
	return m
}

func newField(token theme.Token, dark bool, value string) field {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = len("#rrggbb")
	in.SetWidth(len("#rrggbb"))
	in.SetValue(value)
	s := in.Styles()
	s.Cursor.Blink = false
	in.SetStyles(s)
	return field{token: token, dark: dark, input: in}
}

func (m Model) Init() tea.Cmd { return nil }

// SetDark follows a change of the terminal's background.
func (m *Model) SetDark(dark bool) { m.dark = dark }

// Update handles keys and the host's answers.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.offset = 0
		m.scrollToFocus()
		return m, nil
	case ResultMsg:
		if msg.Err != nil {
			m.status, m.statusErr = "Couldn't apply: "+msg.Err.Error(), true
		} else {
			m.status, m.statusErr = "Applied.", false
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.MouseClickMsg:
		return m.click(msg)
	case tea.MouseWheelMsg:
		return m.wheel(msg), nil
	}
	return m.updateInput(msg)
}

func (m Model) key(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	onButton := m.focus >= len(m.fields)
	switch msg.String() {
	case "esc":
		return m, closeCmd
	case "tab":
		return m, m.focusField(m.focus + 1)
	case "shift+tab":
		return m, m.focusField(m.focus - 1)
	case "down":
		return m, m.focusField(m.nextRow(1))
	case "up":
		return m, m.focusField(m.nextRow(-1))
	case "enter":
		if onButton {
			return m.press(m.focus - len(m.fields))
		}
		return m, m.focusField(m.nextRow(1))
	case "left", "right":
		if onButton {
			step := 1
			if msg.String() == "left" {
				step = -1
			}
			b := (m.focus - len(m.fields) + step + buttonCount) % buttonCount
			return m, m.focusField(len(m.fields) + b)
		}
	}
	if onButton {
		return m, nil
	}
	return m.updateInput(msg)
}

// nextRow moves focus a token up or down, keeping the dark or light
// column. Past the last token it reaches the buttons.
func (m Model) nextRow(step int) int {
	if m.focus >= len(m.fields) {
		if step < 0 {
			return len(m.fields) - 2
		}
		return m.focus
	}
	next := m.focus + 2*step
	if next >= len(m.fields) {
		return len(m.fields) + buttonApply
	}
	return max(next, m.focus%2)
}

func (m Model) updateInput(msg tea.Msg) (Model, tea.Cmd) {
	if m.focus >= len(m.fields) {
		return m, nil
	}
	var cmd tea.Cmd
	m.fields[m.focus].input, cmd = m.fields[m.focus].input.Update(msg)
	return m, cmd
}

func (m *Model) focusField(i int) tea.Cmd {
	total := len(m.fields) + buttonCount
	i = (i + total) % total
	if m.focus < len(m.fields) {
		m.fields[m.focus].input.Blur()
	}
	m.focus = i
	m.scrollToFocus()
	if i < len(m.fields) {
		m.fields[i].input.CursorEnd()
		return m.fields[i].input.Focus()
	}
	return nil
}

func (m Model) press(button int) (Model, tea.Cmd) {
	draft := m.draft()
	switch button {
	case buttonApply:
		m.base = draft
		m.status, m.statusErr = "Applying…", false
		return m, func() tea.Msg { return ApplyMsg{Theme: draft} }
	case buttonCancel:
		return m, closeCmd
	default:
		m.status, m.statusErr = "Copied all values to the clipboard.", false
		return m, tea.SetClipboard(string(draft.YAML()))
	}
}

func closeCmd() tea.Msg { return CloseMsg{} }

// draft is the applied theme with every valid edit. Invalid values
// keep the applied value until they're fixed.
func (m Model) draft() theme.Theme {
	t := m.base
	for i := 0; i < len(m.fields); i += 2 {
		darkField, lightField := m.fields[i], m.fields[i+1]
		v := m.base.Value(darkField.token)
		if s := value(darkField); theme.ValidHex(s) {
			v.Dark = s
		}
		if s := value(lightField); theme.ValidHex(s) {
			v.Light = s
		}
		t = t.With(darkField.token, v)
	}
	return t
}

func value(f field) string { return strings.ToLower(strings.TrimSpace(f.input.Value())) }

// palette colours the creator itself: the applied theme, not the draft,
// so a bad edit can't make the screen unreadable.
func (m Model) palette() style.Palette { return style.New(m.base, m.dark) }
