// Package tracksview is the Tracks window, window 0 of every Tracks
// session: the banner, and the tabs Station, Repositories, Proxy,
// Engines and Settings, switched with Tab, Shift+Tab or a click.
// Station lists the tracks, Repositories manages the repos; the other
// tabs are placeholders until chunk 7. It also hosts the theme creator
// (`t`).
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
	// Repos manages the repositories; ReposErr is why there are none,
	// such as a database that didn't open.
	Repos    source.Repos
	ReposErr error
}

// Model is the Tracks window.
type Model struct {
	version       string
	palette       style.Palette
	apply         ApplyFunc
	source        source.Source
	open, end     TrackFunc
	openURL       func(url string) error
	repoSource    source.Repos
	reposErr      error
	width, height int
	tab           int
	station       station
	repos         repoTab
	creating      bool
	creator       themecreator.Model
}

// New returns the Tracks window for c. It assumes a dark background
// until the terminal reports its colour.
func New(c Config) Model {
	m := Model{version: c.Version, palette: style.New(c.Theme, true), apply: c.Apply, source: c.Tracks,
		station: station{hover: -1}, open: c.Open, end: c.End, openURL: c.OpenURL,
		repoSource: c.Repos, reposErr: c.ReposErr, repos: repoTab{selected: -1, hover: -1}}
	return m.showRepo(-1)
}

// Init asks the terminal for its background colour and reads the
// tracks and repos.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tea.RequestBackgroundColor, m.loadTracks(true), m.loadRepos())
}

// Update handles resizes, the background colour and the theme creator.
// The Tracks window never quits on its own.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.repos.form.setWidth(m.inputWidth())
		m = m.scrollStation().scrollRepos()
	case tracksMsg:
		if !msg.poll {
			return m.setTracks(msg), nil
		}
		return m.setTracks(msg), tea.Tick(refreshEvery, func(time.Time) tea.Msg { return refreshMsg{} })
	case refreshMsg:
		return m, m.loadTracks(true)
	case doneMsg:
		return m.done(msg)
	case reposMsg:
		return m.setRepos(msg), nil
	case suggestMsg:
		return m.suggested(msg), nil
	case repoSavedMsg:
		return m.saved(msg)
	case repoDeletedMsg:
		return m.deleted(msg)
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
	}
	if m.creating {
		var cmd tea.Cmd
		m.creator, cmd = m.creator.Update(msg)
		return m, cmd
	}
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		return m.click(msg.Mouse())
	case tea.MouseMotionMsg:
		return m.hover(msg.Mouse()), nil
	case tea.MouseWheelMsg:
		key := "down"
		if msg.Mouse().Button == tea.MouseWheelUp {
			key = "up"
		}
		switch {
		case m.tab == tabStation:
			next, cmd, _ := m.stationKey(key)
			return next, cmd
		case m.tab == tabRepositories && !m.repos.editing:
			next, cmd, _ := m.repoListKey(key)
			return next, cmd
		}
	case tea.PasteMsg:
		if m.tab == tabRepositories {
			return m.repoPaste(msg)
		}
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.tab == tabRepositories {
		m.repos.notice = notice{}
		if m.repos.editing {
			return m.repoFormKey(msg)
		}
	}
	switch msg.String() {
	case "tab":
		return m.switchTab((m.tab + 1) % len(tabs))
	case "shift+tab":
		return m.switchTab((m.tab + len(tabs) - 1) % len(tabs))
	}
	switch m.tab {
	case tabStation:
		if next, cmd, ok := m.stationKey(msg.String()); ok {
			return next, cmd
		}
	case tabRepositories:
		if next, cmd, ok := m.repoListKey(msg.String()); ok {
			return next, cmd
		}
	}
	if msg.String() == "t" {
		m.creating = true
		m.creator = themecreator.New(m.palette.Theme(), m.palette.Dark())
		var cmd tea.Cmd
		m.creator, cmd = m.creator.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, tea.Batch(m.creator.Init(), cmd)
	}
	return m, nil
}

func (m Model) click(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if i, ok := m.tabAt(mouse.X, mouse.Y); ok {
		if m.tab == tabRepositories {
			return m.request(leave{kind: leaveTab, tab: i})
		}
		return m.switchTab(i)
	}
	switch m.tab {
	case tabStation:
		return m.stationClick(mouse.X, mouse.Y)
	case tabRepositories:
		return m.repoClick(mouse.X, mouse.Y)
	}
	return m, nil
}

func (m Model) hover(mouse tea.Mouse) Model {
	m.station.hover, m.repos.hover = -1, -1
	switch m.tab {
	case tabStation:
		if row, ok := m.rowAt(mouse.X, mouse.Y); ok {
			m.station.hover = row
		}
	case tabRepositories:
		if row, ok := m.repoRowAt(mouse.X, mouse.Y); ok {
			m.repos.hover = row
		}
	}
	return m
}

// switchTab shows tab i. Repositories reads the repos again, since the
// running tracks may have changed.
func (m Model) switchTab(i int) (Model, tea.Cmd) {
	m.tab = i
	if i == tabRepositories {
		return m, m.loadRepos()
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
