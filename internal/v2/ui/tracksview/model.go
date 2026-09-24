// Package tracksview is the Tracks window, window 0 of every Tracks
// session. For now it's a placeholder that shows the build and the
// theme's tokens.
package tracksview

import (
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
)

// Model is the Tracks window.
type Model struct {
	version       string
	palette       style.Palette
	width, height int
}

// New returns the Tracks window for a build of version. It assumes a
// dark background until the terminal reports its colour.
func New(version string) Model {
	return Model{version: version, palette: style.New(theme.Default(), true)}
}

// Init asks the terminal for its background colour.
func (m Model) Init() tea.Cmd { return tea.RequestBackgroundColor }

// Update handles resizes and the background colour. Keys are ignored:
// the Tracks window never quits on its own.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		m.palette = style.New(m.palette.Theme(), msg.IsDark())
	}
	return m, nil
}

// View renders the window full screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Tracks"
	return v
}
