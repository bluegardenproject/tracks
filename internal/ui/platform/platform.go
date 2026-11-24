// Package platform is the Platform: the pane on a track window's left
// that lists the track's trains, each with the Station's status badges
// (see docs/design/trains.md). It reads them from the daemon, again on
// each change the daemon tells of. A click on a train opens its overlay.
package platform

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/style"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// reconnectEvery is how long the Platform waits before watching the
// daemon again after the stream broke; themeEvery how often it reads
// the theme again, which the user may switch or edit meanwhile.
const (
	reconnectEvery = 2 * time.Second
	themeEvery     = 3 * time.Second
)

// Entry is a train as the Platform shows it: its handle, its status
// and an outcome, none for none.
type Entry struct {
	Handle          string
	Status, Outcome track.Status
	// Held says a message of its waits for the track's pointer, held
	// back: Collect messages sends it now.
	Held bool
}

// Config is what the Platform reads and how.
type Config struct {
	Theme theme.Theme
	// Trains lists the track's trains.
	Trains func() ([]Entry, error)
	// Watch calls changed once connected to the daemon and after each
	// change, until the stream breaks.
	Watch func(ctx context.Context, changed func()) error
	// LoadTheme reads the theme the user chose, now.
	LoadTheme func() (theme.Theme, error)
	// Collect sends the track's agent the pointer to its waiting
	// messages now: Collect messages.
	Collect func() error
	// Open opens train handle's overlay; it returns once it closes.
	Open func(handle string)
}

// Model is the Platform.
type Model struct {
	palette       style.Palette
	trains        []Entry
	err           error
	width, height int
	list          func() ([]Entry, error)
	watchFn       func(ctx context.Context, changed func()) error
	changes       chan struct{}
	loadTheme     func() (theme.Theme, error)
	collect       func() error
	// notice says what a click did, or why it couldn't; collecting is
	// a click's call on its way, which other clicks wait for.
	notice     string
	collecting bool
	open       func(handle string)
	// opening says an overlay is being opened: clicks on trains wait
	// for it.
	opening bool
}

type (
	trainsMsg struct {
		trains []Entry
		err    error
	}
	changedMsg struct{}
	themeMsg   struct {
		theme theme.Theme
		err   error
	}
	collectedMsg struct{ err error }
	openedMsg    struct{}
)

// New is the Platform for c.
func New(c Config) Model {
	m := Model{palette: style.New(c.Theme), list: c.Trains, watchFn: c.Watch, loadTheme: c.LoadTheme, collect: c.Collect, open: c.Open}
	if c.Watch != nil {
		m.changes = make(chan struct{}, 1)
	}
	return m
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.load(), m.watch(), m.next(), m.nextTheme()) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case trainsMsg:
		m.trains, m.err = msg.trains, msg.err
		if !m.collecting && !m.anyHeld() {
			// What it said was of a button that's gone.
			m.notice = ""
		}
	case changedMsg:
		return m, tea.Batch(m.load(), m.next())
	case themeMsg:
		if msg.err == nil {
			m.palette = style.New(msg.theme)
		}
		return m, m.nextTheme()
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft && !m.collecting && m.collectAt(msg.X, msg.Y) && m.collect != nil {
			collect := m.collect
			m.notice, m.collecting = "Collecting…", true
			return m, func() tea.Msg { return collectedMsg{collect()} }
		}
		if i, ok := m.trainAt(msg.X, msg.Y); ok && msg.Button == tea.MouseLeft && !m.opening && m.open != nil {
			open, handle := m.open, m.trains[i].Handle
			m.opening = true
			return m, func() tea.Msg { open(handle); return openedMsg{} }
		}
	case openedMsg:
		m.opening = false
	case collectedMsg:
		m.notice, m.collecting = "", false
		if msg.err != nil {
			m.notice = msg.err.Error()
		}
		return m, m.load()
	}
	// No key quits it: the daemon keeps it open while the track has a
	// train, and would only open it again.
	return m, nil
}

// nextTheme reads the theme again after themeEvery.
func (m Model) nextTheme() tea.Cmd {
	if m.loadTheme == nil {
		return nil
	}
	load := m.loadTheme
	return tea.Tick(themeEvery, func(time.Time) tea.Msg {
		t, err := load()
		return themeMsg{t, err}
	})
}

func (m Model) View() tea.View {
	v := tea.NewView(strings.Join(m.lines(), "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// lines are the Platform's frame, the trains in it, filling the pane's
// height.
func (m Model) lines() []string {
	if m.width < 6 || m.height < 3 {
		return nil
	}
	body, _, _ := m.body()
	return widget.Frame(m.palette, "Platform", body, m.width, m.height, theme.BorderDefault)
}

// collectAt reports whether cell x, y of the pane is on a Collect
// messages button: the frame's top border is row 0, its left border and
// padding columns 0 and 1, and the button starts a cell in.
func (m Model) collectAt(x, y int) bool {
	if m.width < 6 || m.height < 3 || y >= m.height-1 {
		return false
	}
	_, buttons, _ := m.body()
	width := widget.NewButton("Collect messages", widget.ButtonAccent).Width()
	for _, row := range buttons {
		if y == row+1 && x >= 3 && x < min(3+width, m.width-2) {
			return true
		}
	}
	return false
}

// trainAt is the train at cell x, y of the pane, on its handle's row or
// its badges'; false for none. The frame's borders are row 0 and the
// first and last columns.
func (m Model) trainAt(x, y int) (int, bool) {
	if m.width < 6 || m.height < 3 || y >= m.height-1 || x < 1 || x >= m.width-1 {
		return 0, false
	}
	_, _, names := m.body()
	for i, row := range names {
		if y == row+1 || y == row+2 {
			return i, true
		}
	}
	return 0, false
}

// anyHeld reports whether a train's messages wait for a held pointer.
func (m Model) anyHeld() bool {
	for _, t := range m.trains {
		if t.Held {
			return true
		}
	}
	return false
}

// body is the frame's inside, the rows of its Collect messages buttons,
// and the row of each train's handle, its badges below it.
func (m Model) body() (body []string, buttons, names []int) {
	inner := m.width - 4
	for _, t := range m.trains {
		handle := t.Handle
		if r := []rune(handle); len(r) > inner {
			handle = string(r[:inner-1]) + "…"
		}
		name := lipgloss.NewStyle().Foreground(m.palette.Color(theme.TextDefault)).Bold(true).Render(handle)
		names = append(names, len(body))
		body = append(body, name)
		badges := widget.Badge(m.palette, t.Status)
		if t.Outcome.Label != "" {
			badges += " " + widget.Badge(m.palette, t.Outcome)
		}
		body = append(body, " "+badges)
		if t.Held {
			buttons = append(buttons, len(body))
			body = append(body, " "+widget.NewButton("Collect messages", widget.ButtonAccent).View(m.palette))
		}
		body = append(body, "")
	}
	switch {
	case m.err != nil:
		body = append(body, m.muted("Couldn't read the trains: "+m.err.Error(), inner)...)
	case len(m.trains) == 0:
		body = append(body, m.muted("No trains.", inner)...)
	}
	if m.notice != "" {
		body = append(body, m.muted(m.notice, inner)...)
	}
	return body, buttons, names
}

// muted is text in muted colour, wrapped to width.
func (m Model) muted(text string, width int) []string {
	return strings.Split(lipgloss.NewStyle().Foreground(m.palette.Color(theme.TextMuted)).Width(max(1, width)).Render(text), "\n")
}

// load reads the trains.
func (m Model) load() tea.Cmd {
	if m.list == nil {
		return nil
	}
	list := m.list
	return func() tea.Msg {
		trains, err := list()
		return trainsMsg{trains, err}
	}
}

// watch follows the daemon's changes, connecting again when the stream
// breaks.
func (m Model) watch() tea.Cmd {
	if m.watchFn == nil {
		return nil
	}
	fn, changes := m.watchFn, m.changes
	return func() tea.Msg {
		for {
			_ = fn(context.Background(), func() {
				select {
				case changes <- struct{}{}:
				default:
				}
			})
			time.Sleep(reconnectEvery)
		}
	}
}

// next waits for the next change.
func (m Model) next() tea.Cmd {
	if m.changes == nil {
		return nil
	}
	changes := m.changes
	return func() tea.Msg {
		<-changes
		return changedMsg{}
	}
}
