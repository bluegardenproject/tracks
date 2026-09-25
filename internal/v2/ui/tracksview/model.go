// Package tracksview is the Tracks window, window 0 of every Tracks
// session: the banner, and tabs whose content is a placeholder for
// now. It also hosts the theme creator.
package tracksview

import (
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/themecreator"
)

// ApplyFunc makes t the session's theme outside this window.
type ApplyFunc func(t theme.Theme, dark bool) error

// Model is the Tracks window.
type Model struct {
	version       string
	palette       style.Palette
	apply         ApplyFunc
	width, height int
	tab           int
	creating      bool
	creator       themecreator.Model
}

// New returns the Tracks window for a build of version, drawn with t.
// It assumes a dark background until the terminal reports its colour.
// apply may be nil.
func New(version string, t theme.Theme, apply ApplyFunc) Model {
	return Model{version: version, palette: style.New(t, true), apply: apply}
}

// Init asks the terminal for its background colour.
func (m Model) Init() tea.Cmd { return tea.RequestBackgroundColor }

// Update handles resizes, the background colour and the theme creator.
// The Tracks window never quits on its own.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		m.palette = style.New(m.palette.Theme(), msg.IsDark())
		m.creator.SetDark(msg.IsDark())
		return m, nil
	case themecreator.ApplyMsg:
		m.palette = style.New(msg.Theme, m.palette.Dark())
		return m, m.applyCmd(msg.Theme)
	case themecreator.CloseMsg:
		m.creating = false
		return m, nil
	case tea.KeyPressMsg:
		if !m.creating {
			switch msg.String() {
			case "tab":
				m.tab = (m.tab + 1) % len(tabs)
				return m, nil
			case "shift+tab":
				m.tab = (m.tab + len(tabs) - 1) % len(tabs)
				return m, nil
			}
		}
		if !m.creating && msg.String() == "t" {
			m.creating = true
			m.creator = themecreator.New(m.palette.Theme(), m.palette.Dark())
			var cmd tea.Cmd
			m.creator, cmd = m.creator.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, tea.Batch(m.creator.Init(), cmd)
		}
	}
	if m.creating {
		var cmd tea.Cmd
		m.creator, cmd = m.creator.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) applyCmd(t theme.Theme) tea.Cmd {
	if m.apply == nil {
		return func() tea.Msg { return themecreator.ResultMsg{} }
	}
	apply, dark := m.apply, m.palette.Dark()
	return func() tea.Msg { return themecreator.ResultMsg{Err: apply(t, dark)} }
}

// View renders the window full screen.
func (m Model) View() tea.View {
	content := m.render()
	if m.creating {
		content = m.creator.View()
	}
	v := tea.NewView(content)
	if m.creating {
		v.MouseMode = tea.MouseModeCellMotion
	}
	v.AltScreen = true
	v.WindowTitle = "Tracks"
	return v
}
