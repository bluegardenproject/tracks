// Package relaystation is the Relay Station: a track's relay, oldest
// first, read only, in a popup over the track (see docs/design/trains.md).
// The daemon's messages reach it as Loaded; it draws and scrolls them.
package relaystation

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/style"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// Keys are the keys it takes, for its hint row.
var Keys = []widget.KeyHelp{
	{Key: "↑/↓", Help: "move"}, {Key: "Enter", Help: "open/fold"}, {Key: "PgUp/PgDn", Help: "scroll"}, {Key: "Esc/q", Help: "close"},
}

// Loaded is the track's relay as the daemon gave it, or why it
// couldn't.
type Loaded struct {
	Messages []relay.Message
	// Waiting are the messages still waiting for their pointer.
	Waiting []int64
	Err     error
}

// wheelStep is how many lines a turn of the mouse wheel scrolls.
const wheelStep = 3

// Model is the Relay Station.
type Model struct {
	palette  style.Palette
	now      func() time.Time
	msgs     []relay.Message
	waiting  map[int64]bool
	open     map[int64]bool // messages unfolded
	selected int            // in msgs; -1 for none
	top      int            // the first body line shown
	width    int
	height   int
	loaded   bool
	err      string
}

// New returns the Relay Station drawn in t, its times read against now.
func New(t theme.Theme, now func() time.Time) Model {
	return Model{palette: style.New(t), now: now, waiting: map[int64]bool{}, open: map[int64]bool{}, selected: -1}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		atEnd := m.atEnd()
		m.width, m.height = msg.Width, msg.Height
		if atEnd {
			m.top = m.maxTop()
		}
		m.top = min(m.top, m.maxTop())
	case Loaded:
		return m.load(msg), nil
	case tea.KeyPressMsg:
		return m.key(msg.String())
	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			m.top = max(0, m.top-wheelStep)
		case tea.MouseWheelDown:
			m.top = min(m.maxTop(), m.top+wheelStep)
		}
	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		if i := m.messageAt(mouse.Y); i >= 0 && mouse.Button == tea.MouseLeft {
			m.selected = i
			return m.toggle(), nil
		}
	}
	return m, nil
}

// load takes in what the daemon gave: the newest message is selected
// and shown while the view was at the end, as when it opens; otherwise
// the selection and the scroll stay.
func (m Model) load(l Loaded) Model {
	if l.Err != nil {
		m.err = l.Err.Error()
		return m
	}
	follow := !m.loaded || m.atEnd() && m.selected == len(m.msgs)-1
	var selectedID int64
	if m.selected >= 0 && m.selected < len(m.msgs) {
		selectedID = m.msgs[m.selected].ID
	}
	m.msgs, m.err, m.loaded = l.Messages, "", true
	m.waiting = map[int64]bool{}
	for _, id := range l.Waiting {
		m.waiting[id] = true
	}
	m.selected = slices.IndexFunc(m.msgs, func(msg relay.Message) bool { return msg.ID == selectedID })
	if follow || m.selected < 0 {
		m.selected = len(m.msgs) - 1
	}
	if follow {
		m.top = m.maxTop()
	}
	m.top = min(m.top, m.maxTop())
	return m
}

func (m Model) key(key string) (tea.Model, tea.Cmd) {
	last := len(m.msgs) - 1
	switch key {
	case "esc", "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		return m.selectAt(max(0, m.selected-1)), nil
	case "down", "j":
		return m.selectAt(min(last, m.selected+1)), nil
	case "home", "g":
		m = m.selectAt(0)
		m.top = 0
	case "end", "G":
		m = m.selectAt(last)
		m.top = m.maxTop()
	case "pgup":
		m.top = max(0, m.top-m.rows())
	case "pgdown", "space":
		m.top = min(m.maxTop(), m.top+m.rows())
	case "enter":
		return m.toggle(), nil
	}
	return m, nil
}

// selectAt selects message i and scrolls it into view.
func (m Model) selectAt(i int) Model {
	if i < 0 || i >= len(m.msgs) {
		return m
	}
	m.selected = i
	return m.reveal()
}

// toggle unfolds the selected message, or folds it again.
func (m Model) toggle() Model {
	if m.selected < 0 || m.selected >= len(m.msgs) {
		return m
	}
	id := m.msgs[m.selected].ID
	m.open[id] = !m.open[id]
	m = m.reveal()
	m.top = min(m.top, m.maxTop())
	return m
}

// reveal scrolls the selected message into view: its head at least,
// all of it when it fits.
func (m Model) reveal() Model {
	l := m.layout()
	if m.selected < 0 || m.selected >= len(l.heads) {
		return m
	}
	head, end := l.heads[m.selected], l.ends[m.selected]
	rows := m.rows()
	switch {
	case head < m.top:
		m.top = head
	case end >= m.top+rows:
		m.top = min(head, end-rows+1)
	}
	return m
}

// messageAt is the message drawn on row y of the popup, -1 for none.
func (m Model) messageAt(y int) int {
	line := y - 1 + m.top
	l := m.layout()
	if y < 1 || y > m.rows() || line >= len(l.owner) {
		return -1
	}
	return l.owner[line]
}

// rows is how many body lines the frame shows: the popup less its
// border and the hint row.
func (m Model) rows() int { return max(0, m.height-3) }

func (m Model) maxTop() int { return max(0, len(m.layout().lines)-m.rows()) }

func (m Model) atEnd() bool { return m.top >= m.maxTop() }
