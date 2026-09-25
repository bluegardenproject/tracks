// Package tracksview is the Tracks window, window 0 of every Tracks
// session: the banner, and the tabs Station, Repositories, Proxy,
// Engines and Settings, switched with Tab, Shift+Tab or a click.
// Station lists the tracks; the other tabs are placeholders until
// chunk 7. It also hosts the theme creator (`t`).
package tracksview

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/themecreator"
)

// ApplyFunc makes t the session's theme outside this window.
type ApplyFunc func(t theme.Theme, dark bool) error

// TrackFunc acts on the track with number.
type TrackFunc func(number int) error

// Config is what the Tracks window is built from. Everything but
// Version and Theme may be nil.
type Config struct {
	Version string
	Theme   theme.Theme
	Apply   ApplyFunc
	Tracks  source.Source
	// Open switches to a track, End closes it.
	Open, End TrackFunc
	OpenURL   func(url string) error
}

// Model is the Tracks window.
type Model struct {
	version       string
	palette       style.Palette
	apply         ApplyFunc
	source        source.Source
	open, end     TrackFunc
	openURL       func(url string) error
	width, height int
	tab           int
	station       station
	creating      bool
	creator       themecreator.Model
}

// New returns the Tracks window for c. It assumes a dark background
// until the terminal reports its colour.
func New(c Config) Model {
	return Model{version: c.Version, palette: style.New(c.Theme, true), apply: c.Apply, source: c.Tracks,
		station: station{hover: -1}, open: c.Open, end: c.End, openURL: c.OpenURL}
}

// Init asks the terminal for its background colour and reads the
// tracks.
func (m Model) Init() tea.Cmd { return tea.Batch(tea.RequestBackgroundColor, m.loadTracks(true)) }

// Update handles resizes, the background colour and the theme creator.
// The Tracks window never quits on its own.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m = m.scrollStation()
	case tracksMsg:
		if !msg.poll {
			return m.setTracks(msg), nil
		}
		return m.setTracks(msg), tea.Tick(refreshEvery, func(time.Time) tea.Msg { return refreshMsg{} })
	case refreshMsg:
		return m, m.loadTracks(true)
	case doneMsg:
		return m.done(msg)
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
	case tea.MouseClickMsg:
		if mouse := msg.Mouse(); !m.creating && mouse.Button == tea.MouseLeft {
			if i, ok := m.tabAt(mouse.X, mouse.Y); ok {
				m.tab = i
				return m, nil
			}
			return m.stationClick(mouse.X, mouse.Y)
		}
	case tea.MouseMotionMsg:
		if !m.creating {
			mouse := msg.Mouse()
			m.station.hover = -1
			if row, ok := m.rowAt(mouse.X, mouse.Y); ok {
				m.station.hover = row
			}
			return m, nil
		}
	case tea.MouseWheelMsg:
		if !m.creating && m.tab == tabStation {
			key := "down"
			if msg.Mouse().Button == tea.MouseWheelUp {
				key = "up"
			}
			next, cmd, _ := m.stationKey(key)
			return next, cmd
		}
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
			if m.tab == tabStation {
				if next, cmd, ok := m.stationKey(msg.String()); ok {
					return next, cmd
				}
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
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	v.WindowTitle = "Tracks"
	return v
}
