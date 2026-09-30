// Package tracksfilter is the Tracks filter popup, opened from Quick
// Access: which tracks Station lists.
package tracksfilter

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// Action is what the popup was closed with.
type Action int

const (
	Cancel Action = iota
	Apply
	Clear
)

// Keys are the keys the popup takes, for its hint row and the Keys list.
var Keys = []widget.KeyHelp{
	{Key: "Tab/↑/↓", Help: "move"}, {Key: "Space", Help: "tick"}, {Key: "Enter", Help: "apply"}, {Key: "Esc", Help: "cancel"},
}

// statusOrder is the track statuses in the popup's order. Closed is
// left out: only archived tracks are, which Archived only picks.
var statusOrder = []track.Status{track.Active, track.ActionRequired, track.Done}

// startedChoices are the Started presets, as the popup names them.
var startedChoices = []struct {
	value track.Started
	label string
}{
	{track.AnyTime, "Any time"}, {track.Today, "Today"}, {track.Last7Days, "Last 7 days"},
	{track.Last30Days, "Last 30 days"}, {track.Between, "Between"},
}

// The kinds of control.
const (
	kindStatus = iota
	kindPR
	kindArchived
	kindStarted
	kindFrom
	kindTo
	kindApply
	kindClear
	kindCancel
)

// control is one focusable thing in the popup; i is its index within
// its kind.
type control struct{ kind, i int }

// controls are every control, in focus order.
var controls = func() []control {
	var out []control
	for i := range statusOrder {
		out = append(out, control{kindStatus, i})
	}
	for i := range track.FilterPRStatuses {
		out = append(out, control{kindPR, i})
	}
	out = append(out, control{kind: kindArchived})
	for i := range startedChoices {
		out = append(out, control{kindStarted, i})
	}
	return append(out, control{kind: kindFrom}, control{kind: kindTo},
		control{kind: kindApply}, control{kind: kindClear}, control{kind: kindCancel})
}()

// Model is the popup. Result says how it was closed.
type Model struct {
	palette       style.Palette
	statuses, prs map[string]bool
	archived      bool
	started       track.Started
	from, to      textinput.Model
	focus, hover  int // in controls; hover -1 for none
	width, height int
	err           string
	action        Action
}

// New returns the popup drawn in t, showing f, the filter that's on.
func New(t theme.Theme, f track.Filter) Model {
	m := Model{palette: style.New(t), statuses: map[string]bool{}, prs: map[string]bool{}, archived: f.Archived,
		started: f.Started, from: widget.NewInput(10, "YYYY-MM-DD"), to: widget.NewInput(10, "YYYY-MM-DD"), hover: -1}
	for _, id := range f.Statuses {
		m.statuses[id] = true
	}
	for _, id := range f.PRStatuses {
		m.prs[id] = true
	}
	m.from.SetValue(f.From)
	m.to.SetValue(f.To)
	return m
}

// Result is how the popup was closed, and the filter to apply.
func (m Model) Result() (Action, track.Filter) { return m.action, m.filter() }

// filter is the filter the controls show.
func (m Model) filter() track.Filter {
	f := track.Filter{Archived: m.archived, Started: m.started}
	for _, s := range statusOrder {
		if m.statuses[s.ID] {
			f.Statuses = append(f.Statuses, s.ID)
		}
	}
	for _, id := range track.FilterPRStatuses {
		if m.prs[id] {
			f.PRStatuses = append(f.PRStatuses, id)
		}
	}
	if f.Started == track.Between {
		f.From, f.To = strings.TrimSpace(m.from.Value()), strings.TrimSpace(m.to.Value())
	}
	return f
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.PasteMsg:
		return m.typed(msg)
	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		if i, ok := m.controlAt(mouse.X, mouse.Y); ok && mouse.Button == tea.MouseLeft {
			m = m.focusOn(i)
			return m.press()
		}
	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		m.hover = -1
		if i, ok := m.controlAt(mouse.X, mouse.Y); ok {
			m.hover = i
		}
	}
	return m, nil
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	c := controls[m.focus]
	typing := c.kind == kindFrom || c.kind == kindTo
	switch key := msg.String(); {
	case key == "esc" || key == "ctrl+c" || key == "q" && !typing:
		m.action = Cancel
		return m, tea.Quit
	case key == "tab" || key == "down" || key == "right" && !typing || key == "j" && !typing:
		if key == "down" || key == "j" {
			return m.focusOn(m.below(1)), nil
		}
		return m.focusOn(min(len(controls)-1, m.focus+1)), nil
	case key == "shift+tab" || key == "up" || key == "left" && !typing || key == "k" && !typing:
		if key == "up" || key == "k" {
			return m.focusOn(m.below(-1)), nil
		}
		return m.focusOn(max(0, m.focus-1)), nil
	case key == "space" && !typing:
		return m.press()
	case key == "enter":
		if c.kind == kindClear || c.kind == kindCancel {
			return m.press()
		}
		return m.apply()
	case typing:
		return m.typed(msg)
	}
	return m, nil
}

// typed hands msg to the focused date field, which picks Between.
func (m Model) typed(msg tea.Msg) (tea.Model, tea.Cmd) {
	in := &m.from
	switch controls[m.focus].kind {
	case kindFrom:
	case kindTo:
		in = &m.to
	default:
		return m, nil
	}
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	m.started, m.err = track.Between, ""
	return m, cmd
}

// focusOn moves the focus to control i, into or out of a date field.
func (m Model) focusOn(i int) Model {
	m.focus = i
	m.from.Blur()
	m.to.Blur()
	switch controls[i].kind {
	case kindFrom:
		m.from.Focus()
	case kindTo:
		m.to.Focus()
	}
	return m
}

// press acts on the focused control.
func (m Model) press() (tea.Model, tea.Cmd) {
	c := controls[m.focus]
	m.err = ""
	switch c.kind {
	case kindStatus:
		id := statusOrder[c.i].ID
		m.statuses[id] = !m.statuses[id]
	case kindPR:
		id := track.FilterPRStatuses[c.i]
		m.prs[id] = !m.prs[id]
	case kindArchived:
		m.archived = !m.archived
	case kindStarted:
		m.started = startedChoices[c.i].value
		if m.started == track.Between {
			return m.focusOn(slices.Index(controls, control{kind: kindFrom})), nil
		}
	case kindApply:
		return m.apply()
	case kindClear:
		m.action = Clear
		return m, tea.Quit
	case kindCancel:
		m.action = Cancel
		return m, tea.Quit
	}
	return m, nil
}

// apply closes the popup with the filter, unless it's wrong.
func (m Model) apply() (tea.Model, tea.Cmd) {
	if err := m.filter().Check(); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.action = Apply
	return m, tea.Quit
}

// View implements tea.Model.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	return v
}
