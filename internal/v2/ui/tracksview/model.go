// Package tracksview is the Tracks window, window 0 of every Tracks
// session: the banner, and the tabs Station, Repositories, Proxy,
// Engines and Settings, switched with Tab, Shift+Tab or a click.
// Station lists the tracks, Repositories manages the repos, Settings
// holds the preferences and the theme creator; the other tabs are
// placeholders until chunk 7.
package tracksview

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/themecreator"
)

// TrackFunc acts on the track with number.
type TrackFunc func(number int) error

// Config is what the Tracks window is built from. Everything but
// Version and Theme may be nil.
type Config struct {
	Version string
	Theme   theme.Theme // the applied theme
	Tracks  source.Source
	// Open switches to a track, End closes it.
	Open, End TrackFunc
	OpenURL   func(url string) error
	// Repos manages the repositories; ReposErr is why there are none,
	// such as a database that didn't open.
	Repos    source.Repos
	ReposErr error
	// Themes lists, chooses and saves themes; ThemesDir is where users
	// put theme files.
	Themes    source.Themes
	ThemesDir string
	// About is what the Settings tab's About section lists, label and
	// value.
	About [][2]string
}

// Model is the Tracks window.
type Model struct {
	version       string
	palette       style.Palette
	source        source.Source
	open, end     TrackFunc
	openURL       func(url string) error
	repoSource    source.Repos
	reposErr      error
	themeSource   source.Themes
	themesDir     string
	aboutFacts    [][2]string
	width, height int
	tab           int
	station       station
	repos         repoTab
	settings      settingsTab
}

// New returns the Tracks window for c.
func New(c Config) Model {
	m := Model{version: c.Version, palette: style.New(c.Theme), source: c.Tracks,
		station: station{hover: -1, hoverButton: -1}, open: c.Open, end: c.End, openURL: c.OpenURL,
		repoSource: c.Repos, reposErr: c.ReposErr, repos: repoTab{selected: -1, hover: -1, hoverField: -1},
		themeSource: c.Themes, themesDir: c.ThemesDir, aboutFacts: c.About, settings: newSettingsTab(c.Theme)}
	return m.showRepo(-1)
}

// Init reads the tracks, repos and themes.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadTracks(true), m.loadRepos(), m.loadThemes())
}

// Update handles resizes, data and input. The Tracks window never quits
// on its own.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.repos.form.setWidth(m.inputWidth())
		m.settings.creator.SetSize(m.sectionWidth(), m.sectionHeight())
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
	case themesMsg:
		return m.setThemes(msg), nil
	case chosenMsg:
		return m.chosen(msg), nil
	case themecreator.SaveMsg, themecreator.CreateMsg, themecreator.SavedMsg, themecreator.DoneMsg, themecreator.StayMsg, themecreator.LoadMsg:
		return m.creatorMsg(msg)
	}
	if m.settings.picker != nil {
		switch msg := msg.(type) {
		case tea.MouseClickMsg, tea.MouseMotionMsg, tea.MouseWheelMsg:
			return m.pickerMouse(msg)
		case tea.KeyPressMsg:
			return m.pickerKey(msg.String())
		case tea.PasteMsg:
			return m, nil
		}
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
		case m.tab == tabSettings:
			return m.settingsWheel(msg)
		}
	case tea.PasteMsg:
		switch {
		case m.tab == tabRepositories:
			return m.repoPaste(msg)
		case m.creatorFocused():
			return m.creatorMsg(msg)
		}
	case tea.KeyPressMsg:
		return m.key(msg)
	default:
		switch {
		case m.creatorFocused():
			return m.creatorMsg(msg)
		case m.tab == tabRepositories && m.repos.editing:
			return m.repoPaste(msg)
		}
	}
	return m, nil
}

// creatorFocused reports whether the theme creator has focus.
func (m Model) creatorFocused() bool {
	return m.tab == tabSettings && m.settings.section == sectionCreator && m.settings.editing
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.tab {
	case tabRepositories:
		m.repos.notice = notice{}
		if m.repos.editing {
			return m.repoFormKey(msg)
		}
	case tabSettings:
		m.settings.notice = notice{}
		if m.settings.editing {
			return m.settingsKey(msg)
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
	case tabSettings:
		if next, cmd, ok := m.settingsListKey(msg.String()); ok {
			return next, cmd
		}
	}
	return m, nil
}

func (m Model) click(mouse tea.Mouse) (tea.Model, tea.Cmd) {
	if mouse.Button != tea.MouseLeft {
		return m, nil
	}
	if i, ok := m.tabAt(mouse.X, mouse.Y); ok {
		switch m.tab {
		case tabRepositories:
			return m.request(leave{kind: leaveTab, tab: i})
		case tabSettings:
			return m.settingsRequest(settingsLeave{section: -1, tab: i})
		}
		return m.switchTab(i)
	}
	switch m.tab {
	case tabStation:
		return m.stationClick(mouse.X, mouse.Y)
	case tabRepositories:
		return m.repoClick(mouse.X, mouse.Y)
	case tabSettings:
		return m.settingsClick(mouse.X, mouse.Y)
	}
	return m, nil
}

func (m Model) hover(mouse tea.Mouse) Model {
	m.station.hover, m.repos.hover = -1, -1
	m.station.hoverButton, m.repos.hoverField, m.repos.hoverNew = -1, -1, false
	switch m.tab {
	case tabStation:
		if row, ok := m.rowAt(mouse.X, mouse.Y); ok {
			m.station.hover = row
		}
		if id, ok := m.buttonAt(mouse.X, mouse.Y); ok {
			m.station.hoverButton = id
		}
	case tabRepositories:
		if row, ok := m.repoRowAt(mouse.X, mouse.Y); ok {
			m.repos.hover = row
		}
		m.repos.hoverNew = m.onNewRepo(mouse.X, mouse.Y)
		if field, ok := m.repoFieldAt(mouse.X, mouse.Y); ok {
			m.repos.hoverField = field
		}
	case tabSettings:
		return m.settingsHover(mouse.X, mouse.Y)
	}
	return m
}

// switchTab shows tab i. Repositories reads the repos again, since the
// running tracks may have changed, and Settings the theme files.
func (m Model) switchTab(i int) (Model, tea.Cmd) {
	m.tab = i
	switch i {
	case tabRepositories:
		return m, m.loadRepos()
	case tabSettings:
		return m, m.loadThemes()
	}
	return m, nil
}

// View renders the window full screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.withPicker(m.render()))
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	v.WindowTitle = "Tracks"
	return v
}
