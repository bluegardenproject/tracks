// Package quickaccess is the Quick Access popup: a short list of
// things to open from any window, behind Ctrl+b q.
package quickaccess

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/style"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// Entry is something Quick Access opens; Key opens it directly.
type Entry struct{ ID, Label, Key string }

// NewTrack opens the New track form, TracksFilter the Tracks filter;
// CloseTracks closes Tracks, once confirmed.
const (
	NewTrack     = "new-track"
	TracksFilter = "tracks-filter"
	CloseTracks  = "close-tracks"
)

var entries = []Entry{
	{ID: NewTrack, Label: "New track", Key: "n"},
	{ID: TracksFilter, Label: "Tracks filter", Key: "f"},
	{ID: CloseTracks, Label: "Close Tracks", Key: "c"},
}

// Keys are the keys Quick Access takes, for its hint row and the Keys
// list.
var Keys = []widget.KeyHelp{{Key: "↑/↓", Help: "select"}, {Key: "Enter", Help: "open"}, {Key: "Esc", Help: "close"}}

// Width is the popup's width in cells.
const Width = 40

// Height is the popup's height: the entries in their frame and the
// hint row.
func Height() int { return len(entries) + 3 }

// Model is the popup. Chosen reports what to open once it quits.
type Model struct {
	palette       style.Palette
	cursor, hover int
	width, height int
	chosen        string
}

// New returns the popup drawn in t.
func New(t theme.Theme) Model { return Model{palette: style.New(t), hover: -1} }

// Chosen is the ID of the entry picked, "" when closed without one.
func (m Model) Chosen() string { return m.chosen }

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return m.key(msg.String())
	case tea.MouseClickMsg:
		mouse := msg.Mouse()
		if mouse.Button != tea.MouseLeft {
			return m, nil
		}
		i, ok := m.entryAt(mouse.X, mouse.Y)
		if !ok {
			return m, nil
		}
		return m.choose(i)
	case tea.MouseMotionMsg:
		mouse := msg.Mouse()
		m.hover = -1
		if i, ok := m.entryAt(mouse.X, mouse.Y); ok {
			m.hover = i
		}
	}
	return m, nil
}

func (m Model) key(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
	case "down", "j":
		m.cursor = min(len(entries)-1, m.cursor+1)
	case "enter", "space":
		return m.choose(m.cursor)
	case "esc", "q", "ctrl+c":
		return m, tea.Quit
	default:
		for i, e := range entries {
			if e.Key == key {
				return m.choose(i)
			}
		}
	}
	return m, nil
}

func (m Model) choose(i int) (tea.Model, tea.Cmd) {
	m.chosen = entries[i].ID
	return m, tea.Quit
}

// entryAt is the entry drawn at cell x, y.
func (m Model) entryAt(x, y int) (int, bool) {
	i := y - 1
	if x < 2 || x >= m.width-2 || i < 0 || i >= len(entries) {
		return 0, false
	}
	return i, true
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
	inner := m.width - 4
	rows := make([]string, len(entries))
	for i, e := range entries {
		rows[i] = m.row(e, inner, i == m.hover, i == m.cursor)
	}
	lines := widget.Frame(m.palette, "Quick Access", rows, m.width, min(m.height-1, len(entries)+2), theme.OverlayBorderFocus)
	fg := func(t theme.Token) lipgloss.Style { return lipgloss.NewStyle().Foreground(m.palette.Color(t)) }
	lines = append(lines, " "+widget.Hints(fg(theme.TextAccent), fg(theme.TextFaint), Keys))
	return strings.Join(lines, "\n")
}

// row draws e as a list item inner cells wide, its key on the right.
func (m Model) row(e Entry, inner int, hover, active bool) string {
	bg, text, key := theme.ListItemBgDefault, theme.ListItemTextDefault, theme.TextAccent
	switch {
	case active:
		bg, text, key = theme.ListItemBgActive, theme.ListItemTextActive, theme.ListItemTextActive
	case hover:
		bg, text, key = theme.ListItemBgHover, theme.ListItemTextHover, theme.ListItemTextHover
	}
	s := lipgloss.NewStyle().Background(m.palette.Color(bg))
	gap := max(1, inner-lipgloss.Width(e.Label)-lipgloss.Width(e.Key)-2)
	return s.Foreground(m.palette.Color(text)).Bold(active).Render(" "+e.Label+strings.Repeat(" ", gap)) +
		s.Foreground(m.palette.Color(key)).Render(e.Key+" ")
}
