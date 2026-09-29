// Package tracksview is the Tracks window, window 0 of every Tracks
// session: the banner, and the tabs Station, Repositories, Proxy,
// Engines and Settings, switched with Tab, Shift+Tab or a click.
// Station lists the tracks, Repositories manages the repos, Engines sets
// up the agent CLIs, Settings holds the preferences and the theme
// creator; Proxy is a placeholder until chunk 7.
package tracksview

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/themecreator"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// TrackFunc acts on the track with number.
type TrackFunc func(number int) error

// Config is what the Tracks window is built from. Everything but
// Version and Theme may be nil.
type Config struct {
	Version string
	Theme   theme.Theme // the applied theme
	Tracks  source.Source
	// Watch calls changed once connected to the daemon and after each
	// change to the tracks, until the connection breaks; nil reads the
	// tracks once.
	Watch func(ctx context.Context, changed func()) error
	// Open switches to a track, End closes it.
	Open, End TrackFunc
	// Resume starts an ended track again. Unsaved is Clean's check: the
	// work in an ended track's worktrees that exists nowhere else.
	Resume ResumeFunc
	// Archive takes an ended track out of Station, as Clean removing
	// its worktrees first.
	Unsaved func(id string) ([]string, error)
	Clean   CleanFunc
	Archive CleanFunc
	// Unarchive puts an archived track back in Station; SetFilter puts
	// Station under a filter, the zero Filter clearing it.
	Unarchive func(id string) error
	SetFilter func(track.Filter) error
	// NewTrack opens the New track form and returns once it closes.
	NewTrack func() error
	OpenURL  func(url string) error
	// Repos manages the repositories; ReposErr is why there are none,
	// such as a database that didn't open.
	Repos    source.Repos
	ReposErr error
	// Themes lists, chooses and saves themes; ThemesDir is where users
	// put theme files.
	Themes    source.Themes
	ThemesDir string
	// Engines keeps the engines' settings and asks their CLIs;
	// TrackTypes each track type's default agent and model.
	Engines    source.Engines
	TrackTypes source.TrackTypes
	// History keeps Tracks History, the auto-archive settings.
	History source.History
	// About is what the Settings tab's About section lists, label and
	// value.
	About [][2]string
}

// Model is the Tracks window.
type Model struct {
	version       string
	palette       style.Palette
	source        source.Source
	watchFn       func(ctx context.Context, changed func()) error
	changes       chan watchMsg // from watchFn, nil without it
	open, end     TrackFunc
	resume        ResumeFunc
	unsaved       func(id string) ([]string, error)
	cleanFn       CleanFunc
	archiveFn     CleanFunc
	unarchive     func(id string) error
	setFilter     func(track.Filter) error
	newTrack      func() error
	openURL       func(url string) error
	repoSource    source.Repos
	reposErr      error
	themeSource   source.Themes
	themesDir     string
	engineSource  source.Engines
	typeSource    source.TrackTypes
	historySource source.History
	aboutFacts    [][2]string
	width, height int
	tab           int
	station       station
	repos         repoTab
	settings      settingsTab
	engines       enginesTab
	// picker, when set, is open over the window; pickerFor is what it
	// chooses, and pickerEngine the engine for pickModel and
	// pickTypeModel.
	picker       *widget.Picker
	pickerFor    int
	pickerEngine string
	// noticeSeq counts each tab's notices, so only the latest one's
	// clock ends it.
	noticeSeq [tabSettings + 1]int
}

// New returns the Tracks window for c.
func New(c Config) Model {
	m := Model{version: c.Version, palette: style.New(c.Theme), source: c.Tracks,
		station: station{hover: -1, hoverButton: -1}, open: c.Open, end: c.End, newTrack: c.NewTrack, openURL: c.OpenURL,
		resume: c.Resume, unsaved: c.Unsaved, cleanFn: c.Clean, archiveFn: c.Archive, unarchive: c.Unarchive, setFilter: c.SetFilter,
		repoSource: c.Repos, reposErr: c.ReposErr, repos: repoTab{selected: -1, hover: -1, hoverField: -1},
		themeSource: c.Themes, themesDir: c.ThemesDir, aboutFacts: c.About, settings: newSettingsTab(c.Theme),
		engineSource: c.Engines, typeSource: c.TrackTypes, historySource: c.History, engines: newEnginesTab()}
	if c.Watch != nil {
		m.watchFn, m.changes = c.Watch, make(chan watchMsg, 1)
	}
	return m.showRepo(-1)
}

// Init reads the tracks, repos and themes, and follows the tracks'
// changes.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadTracks(), m.watch(), m.nextChange(), m.loadRepos(), m.loadThemes(), m.loadTypes(), m.loadHistory())
}

// Update handles resizes, data and input. The Tracks window never quits
// on its own.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(noticeExpiredMsg); ok {
		return m.expire(msg), nil
	}
	before := m.notices()
	next, cmd := m.update(msg)
	next, clock := next.(Model).timeNotices(before)
	return next, tea.Batch(cmd, clock)
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.repos.form.setWidth(m.inputWidth())
		m.settings.creator.SetSize(m.sectionWidth(), m.sectionHeight())
		m = m.scrollStation().scrollRepos()
	case tracksMsg:
		return m.setTracks(msg), nil
	case watchMsg:
		return m.watched(msg)
	case doneMsg:
		return m.done(msg)
	case resumeEvent:
		return m.resumed(msg)
	case checkedMsg:
		return m.checked(msg), nil
	case archivedMsg:
		return m.archived(msg)
	case cleanedMsg:
		return m.cleaned(msg)
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
	case enginesMsg:
		return m.setEngines(msg), nil
	case engineCheckedMsg:
		return m.engineChecked(msg)
	case engineModelsMsg:
		return m.engineModels(msg), nil
	case engineMCPMsg:
		return m.engineMCP(msg), nil
	case enginesSavedMsg:
		return m.enginesSaved(msg)
	case typesMsg:
		return m.setTypes(msg), nil
	case historyMsg:
		return m.setHistory(msg), nil
	case historySavedMsg:
		return m.historySaved(msg)
	case typesSavedMsg:
		return m.typesSaved(msg)
	case themecreator.SaveMsg, themecreator.CreateMsg, themecreator.SavedMsg, themecreator.DoneMsg, themecreator.StayMsg, themecreator.LoadMsg:
		return m.creatorMsg(msg)
	}
	if m.picker != nil {
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
		case m.tab == tabEngines:
			step := 3
			if key == "up" {
				step = -3
			}
			return m.scrollEnginesBy(step), nil
		case m.tab == tabSettings:
			return m.settingsWheel(msg)
		}
	case tea.PasteMsg:
		switch {
		case m.tab == tabRepositories:
			return m.repoPaste(msg)
		case m.tab == tabEngines:
			return m.enginesPaste(msg)
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
		case m.tab == tabEngines:
			return m.enginesPaste(msg)
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
	case tabEngines:
		m.engines.notice = notice{}
		if m.engines.editing {
			return m.enginesKey(msg)
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
	case tabEngines:
		if next, cmd, ok := m.enginesListKey(msg.String()); ok {
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
	if m.onNoticeClose(mouse.X, mouse.Y) {
		*m.noticeOf(m.tab) = notice{}
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
	case tabEngines:
		return m.enginesClick(mouse.X, mouse.Y)
	case tabSettings:
		return m.settingsClick(mouse.X, mouse.Y)
	}
	return m, nil
}

func (m Model) hover(mouse tea.Mouse) Model {
	m.station.hover, m.repos.hover = -1, -1
	m.station.hoverButton, m.repos.hoverField, m.repos.hoverNew = -1, -1, false
	m.station.hoverAdd, m.station.hoverClear = false, false
	m.engines.hover = engineControl{}
	switch m.tab {
	case tabStation:
		if row, ok := m.rowAt(mouse.X, mouse.Y); ok {
			m.station.hover = row
		}
		if id, ok := m.buttonAt(mouse.X, mouse.Y); ok {
			m.station.hoverButton = id
		}
		m.station.hoverAdd = m.onAddTrack(mouse.X, mouse.Y)
		m.station.hoverClear = m.onClearFilter(mouse.X, mouse.Y)
	case tabRepositories:
		if row, ok := m.repoRowAt(mouse.X, mouse.Y); ok {
			m.repos.hover = row
		}
		m.repos.hoverNew = m.onNewRepo(mouse.X, mouse.Y)
		if field, ok := m.repoFieldAt(mouse.X, mouse.Y); ok {
			m.repos.hoverField = field
		}
	case tabEngines:
		return m.enginesHover(mouse.X, mouse.Y)
	case tabSettings:
		return m.settingsHover(mouse.X, mouse.Y)
	}
	return m
}

// switchTab shows tab i. Repositories reads the repos again, since the
// running tracks may have changed, Engines looks for the CLIs, and
// Settings reads the theme files.
func (m Model) switchTab(i int) (Model, tea.Cmd) {
	m.tab = i
	switch i {
	case tabRepositories:
		return m, m.loadRepos()
	case tabEngines:
		m.engines.editing = false
		m.engines.input.Blur()
		return m.loadEngines()
	case tabSettings:
		return m, tea.Batch(m.loadThemes(), m.loadTypes(), m.loadHistory())
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
