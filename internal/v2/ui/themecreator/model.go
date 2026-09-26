// Package themecreator edits a theme: every token with a preview and
// its value. It's a pane the host sizes and places; the host saves
// themes and says when the creator should let go.
package themecreator

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// Messages to the host.
type (
	// SaveMsg asks the host to save Theme over its file. The host
	// answers with SavedMsg.
	SaveMsg struct{ Theme theme.Theme }
	// CreateMsg asks the host to save Theme as a new theme called
	// Name. The host answers with SavedMsg.
	CreateMsg struct {
		Name  string
		Theme theme.Theme
	}
	// DoneMsg means the creator can be left: nothing is unsaved.
	DoneMsg struct{}
	// StayMsg means the user chose to keep editing instead of leaving.
	StayMsg struct{}
	// LoadMsg asks the host to offer the themes to load. It answers
	// by calling Load with the choice.
	LoadMsg struct{}
)

// SavedMsg is the host's answer to SaveMsg and CreateMsg.
type SavedMsg struct {
	Theme theme.Theme
	Err   error
}

// mode is what the creator shows below its title.
type mode int

const (
	modeEdit  mode = iota // the token list
	modeName              // naming a new theme
	modeLeave             // asking about unsaved changes
)

// button is a button in the row below the list.
type button int

const (
	buttonLoad button = iota
	buttonSave
	buttonSaveAs
	buttonCopy
	buttonCreate
	buttonCancelName
	buttonLeaveSave
	buttonDiscard
	buttonStay
)

var buttonLabels = map[button]string{
	buttonLoad: "Load", buttonSave: "Save", buttonSaveAs: "Save as new", buttonCopy: "Copy all",
	buttonCreate: "Create", buttonCancelName: "Cancel",
	buttonLeaveSave: "Save", buttonDiscard: "Discard", buttonStay: "Cancel",
}

// field is one token's value.
type field struct {
	token theme.Token
	input textinput.Model
}

// Model is the creator. Focus runs through the fields, in the order of
// theme.All, then the buttons of the current mode.
type Model struct {
	applied       theme.Theme // colours the creator itself
	edit          theme.Theme // the theme being edited, as saved
	fields        []field
	mode          mode
	focus         int
	focused       bool // the host gave the creator focus
	offset        int  // first visible line of the token list
	name          textinput.Model
	saving        bool // a save was asked for before leaving
	sampleHover   int  // the example button under the mouse, -1 for none
	buttonHover   int  // the index of the button under the mouse, -1 for none
	width, height int
	status        string
	statusErr     bool
}

// New returns a creator editing t, drawn in the colours of applied.
func New(t, applied theme.Theme) Model {
	m := Model{applied: applied, name: widget.NewInput(64, "My theme"), sampleHover: -1, buttonHover: -1}
	m.Open(t)
	return m
}

// Open starts editing t, dropping any edits.
func (m *Model) Open(t theme.Theme) {
	m.edit, m.mode, m.focus, m.offset, m.saving = t, modeEdit, 0, 0, false
	m.fields = m.fields[:0]
	for _, token := range theme.All {
		in := widget.NewInput(len("#rrggbb"), "")
		in.SetWidth(len("#rrggbb"))
		in.SetValue(t.Value(token))
		m.fields = append(m.fields, field{token: token, input: in})
	}
	m.setFocus(0)
}

// SetApplied changes the colours the creator is drawn in.
func (m *Model) SetApplied(t theme.Theme) { m.applied = t }

// SetSize sets the pane's size in cells.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
	m.scrollToFocus()
}

// Focus and Blur follow the host: only a focused creator shows a
// cursor.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	return m.setFocus(m.focus)
}

func (m *Model) Blur() {
	m.focused = false
	m.blurAll()
}

// Load starts editing t, the host's answer to LoadMsg.
func (m *Model) Load(t theme.Theme) tea.Cmd {
	m.Open(t)
	m.status, m.statusErr = "Loaded "+t.DisplayName+".", false
	return m.setFocus(0)
}

// Editing is the theme being edited.
func (m Model) Editing() theme.Theme { return m.edit }

// Dirty reports unsaved edits.
func (m Model) Dirty() bool {
	for _, f := range m.fields {
		if s := value(f); theme.ValidHex(s) && s != m.edit.Value(f.token) {
			return true
		}
	}
	return false
}

// Leave asks to let go. It reports true when nothing is unsaved;
// otherwise the creator asks, and answers later with DoneMsg or
// StayMsg.
func (m *Model) Leave() bool {
	if !m.Dirty() {
		return true
	}
	m.mode, m.saving = modeLeave, false
	m.setFocus(0)
	return false
}

// Update handles keys, clicks and the host's answers.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case SavedMsg:
		return m.saved(msg)
	case tea.KeyPressMsg:
		m.status = ""
		return m.key(msg)
	case tea.PasteMsg:
		return m.updateInput(msg)
	case tea.MouseClickMsg:
		m.status = ""
		return m.click(msg)
	case tea.MouseWheelMsg:
		return m.wheel(msg), nil
	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		m.sampleHover = m.sampleAt(mouse.X, mouse.Y)
		m.buttonHover = m.buttonAt(mouse.X, mouse.Y)
		return m, nil
	}
	return m.updateInput(msg)
}

func (m Model) saved(msg SavedMsg) (Model, tea.Cmd) {
	if msg.Err != nil {
		m.status, m.statusErr = "Couldn't save: "+msg.Err.Error(), true
		if m.mode == modeLeave {
			m.mode, m.saving = modeEdit, false
		}
		return m, nil
	}
	leaving := m.saving
	m.Open(msg.Theme)
	m.status, m.statusErr = "Saved "+msg.Theme.DisplayName+".", false
	if leaving {
		return m, func() tea.Msg { return DoneMsg{} }
	}
	return m, m.setFocus(m.firstButton())
}

// buttons are the current mode's buttons, in focus order.
func (m Model) buttons() []button {
	switch m.mode {
	case modeName:
		return []button{buttonCreate, buttonCancelName}
	case modeLeave:
		return []button{buttonLeaveSave, buttonDiscard, buttonStay}
	}
	b := []button{buttonLoad}
	if !m.edit.BuiltIn && m.Dirty() {
		b = append(b, buttonSave)
	}
	return append(b, buttonSaveAs, buttonCopy)
}

// focusables is how many things take focus in the current mode: the
// fields or the name input, then the buttons.
func (m Model) focusables() int {
	switch m.mode {
	case modeName:
		return 1 + len(m.buttons())
	case modeLeave:
		return len(m.buttons())
	}
	return len(m.fields) + len(m.buttons())
}

// firstButton is the focus index of the first button.
func (m Model) firstButton() int {
	switch m.mode {
	case modeName:
		return 1
	case modeLeave:
		return 0
	}
	return len(m.fields)
}

// focusedButton is the button with focus, if any.
func (m Model) focusedButton() (button, bool) {
	i := m.focus - m.firstButton()
	b := m.buttons()
	if i < 0 || i >= len(b) {
		return 0, false
	}
	return b[i], true
}

func (m Model) key(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch m.mode {
	case modeLeave:
		switch msg.String() {
		case "s":
			return m.press(buttonLeaveSave)
		case "d":
			return m.press(buttonDiscard)
		case "c", "esc":
			return m.press(buttonStay)
		}
	case modeName:
		if msg.String() == "esc" {
			return m.press(buttonCancelName)
		}
	}
	b, onButton := m.focusedButton()
	switch msg.String() {
	case "ctrl+c":
		return m.copyValue()
	case "esc":
		if m.Leave() {
			return m, func() tea.Msg { return DoneMsg{} }
		}
		return m, nil
	case "tab":
		return m, m.setFocus(m.focus + 1)
	case "shift+tab":
		return m, m.setFocus(m.focus - 1)
	case "down":
		if m.mode == modeEdit && !onButton {
			return m, m.setFocus(m.focus + 1)
		}
	case "up":
		if m.mode == modeEdit && m.focus > 0 {
			return m, m.setFocus(min(m.focus, m.firstButton()) - 1)
		}
	case "left", "right":
		if onButton {
			step := 1
			if msg.String() == "left" {
				step = -1
			}
			n := len(m.buttons())
			i := (m.focus - m.firstButton() + step + n) % n
			return m, m.setFocus(m.firstButton() + i)
		}
	case "enter":
		if onButton {
			return m.press(b)
		}
		if m.mode == modeName {
			return m.press(buttonCreate)
		}
		return m, m.setFocus(m.focus + 1)
	}
	if onButton {
		return m, nil
	}
	return m.updateInput(msg)
}

func (m Model) press(b button) (Model, tea.Cmd) {
	switch b {
	case buttonLoad:
		if m.Dirty() {
			m.status, m.statusErr = "Save or undo your edits first.", true
			return m, nil
		}
		return m, func() tea.Msg { return LoadMsg{} }
	case buttonSave:
		return m, m.save()
	case buttonSaveAs:
		m.mode = modeName
		m.name.SetValue("")
		return m, m.setFocus(0)
	case buttonCopy:
		m.status, m.statusErr = "Copied the theme file to the clipboard.", false
		d := m.draft()
		return m, tea.SetClipboard(string(d.YAML()))
	case buttonCreate:
		name := strings.TrimSpace(m.name.Value())
		if name == "" {
			m.status, m.statusErr = "Enter a display name.", true
			return m, nil
		}
		d := m.draft()
		return m, func() tea.Msg { return CreateMsg{Name: name, Theme: d} }
	case buttonCancelName:
		if m.saving {
			m.mode = modeLeave
			return m, m.setFocus(0)
		}
		m.mode = modeEdit
		return m, m.setFocus(m.firstButton())
	case buttonLeaveSave:
		m.saving = true
		if m.edit.BuiltIn {
			m.mode = modeName
			m.name.SetValue("")
			return m, m.setFocus(0)
		}
		return m, m.save()
	case buttonDiscard:
		m.Open(m.edit)
		return m, func() tea.Msg { return DoneMsg{} }
	case buttonStay:
		m.mode, m.saving = modeEdit, false
		return m, tea.Batch(m.setFocus(0), func() tea.Msg { return StayMsg{} })
	}
	return m, nil
}

// copyValue puts the focused input's value on the clipboard.
func (m Model) copyValue() (Model, tea.Cmd) {
	var v string
	switch {
	case m.mode == modeName && m.focus == 0:
		v = m.name.Value()
	case m.mode == modeEdit && m.focus < len(m.fields):
		v = m.fields[m.focus].input.Value()
	default:
		return m, nil
	}
	m.status, m.statusErr = "Copied "+v+" to the clipboard.", false
	return m, tea.SetClipboard(v)
}

func (m Model) save() tea.Cmd {
	d := m.draft()
	return func() tea.Msg { return SaveMsg{Theme: d} }
}

func (m Model) updateInput(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch {
	case !m.focused:
	case m.mode == modeName && m.focus == 0:
		m.name, cmd = m.name.Update(msg)
	case (m.mode == modeEdit) && m.focus < len(m.fields):
		m.fields[m.focus].input, cmd = m.fields[m.focus].input.Update(msg)
	}
	return m, cmd
}

// setFocus moves focus to i, wrapping, and focuses its input.
func (m *Model) setFocus(i int) tea.Cmd {
	m.blurAll()
	n := m.focusables()
	if n == 0 {
		return nil
	}
	m.focus = (i + n) % n
	m.scrollToFocus()
	if !m.focused {
		return nil
	}
	switch {
	case m.mode == modeName && m.focus == 0:
		m.name.CursorEnd()
		return m.name.Focus()
	case m.mode == modeEdit && m.focus < len(m.fields):
		m.fields[m.focus].input.CursorEnd()
		return m.fields[m.focus].input.Focus()
	}
	return nil
}

func (m *Model) blurAll() {
	for i := range m.fields {
		m.fields[i].input.Blur()
	}
	m.name.Blur()
}

// draft is the edited theme with every valid value. Invalid values keep
// the saved value until they're fixed.
func (m Model) draft() theme.Theme {
	t := m.edit
	for _, f := range m.fields {
		if s := value(f); theme.ValidHex(s) {
			t = t.With(f.token, s)
		}
	}
	return t
}

func value(f field) string { return strings.ToLower(strings.TrimSpace(f.input.Value())) }

// palette colours the creator itself: the applied theme, not the draft,
// so a bad edit can't make the pane unreadable.
func (m Model) palette() style.Palette { return style.New(m.applied) }
