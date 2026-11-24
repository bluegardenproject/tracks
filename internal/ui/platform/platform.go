// Package platform is the Platform: the pane on a track window's left
// that lists the track's trains, each with the Station's status badges
// (see docs/design/trains.md). It reads them from the daemon, again on
// each change the daemon tells of.
package platform

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/train"
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

// Config is what the Platform reads and how.
type Config struct {
	Theme theme.Theme
	// Trains lists the track's trains.
	Trains func() ([]rpc.Train, error)
	// Watch calls changed once connected to the daemon and after each
	// change, until the stream breaks.
	Watch func(ctx context.Context, changed func()) error
	// LoadTheme reads the theme the user chose, now.
	LoadTheme func() (theme.Theme, error)
}

// Model is the Platform.
type Model struct {
	palette       style.Palette
	trains        []rpc.Train
	err           error
	width, height int
	list          func() ([]rpc.Train, error)
	watchFn       func(ctx context.Context, changed func()) error
	changes       chan struct{}
	loadTheme     func() (theme.Theme, error)
}

type (
	trainsMsg struct {
		trains []rpc.Train
		err    error
	}
	changedMsg struct{}
	themeMsg   struct {
		theme theme.Theme
		err   error
	}
)

// New is the Platform for c.
func New(c Config) Model {
	m := Model{palette: style.New(c.Theme), list: c.Trains, watchFn: c.Watch, loadTheme: c.LoadTheme}
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
	case changedMsg:
		return m, tea.Batch(m.load(), m.next())
	case themeMsg:
		if msg.err == nil {
			m.palette = style.New(msg.theme)
		}
		return m, m.nextTheme()
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
	return v
}

// lines are the Platform's frame, the trains in it, filling the pane's
// height.
func (m Model) lines() []string {
	if m.width < 6 || m.height < 3 {
		return nil
	}
	inner := m.width - 4
	var body []string
	for _, t := range m.trains {
		name := lipgloss.NewStyle().Foreground(m.palette.Color(theme.TextDefault)).Bold(true).Render(t.Handle)
		body = append(body, name)
		badges := widget.Badge(m.palette, train.StatusOf(t.StatusID))
		if t.OutcomeID != "" {
			badges += " " + widget.Badge(m.palette, train.StatusOf(t.OutcomeID))
		}
		body = append(body, " "+badges, "")
	}
	switch {
	case m.err != nil:
		body = append(body, m.muted("Couldn't read the trains: "+m.err.Error(), inner)...)
	case len(m.trains) == 0:
		body = append(body, m.muted("No trains.", inner)...)
	}
	return widget.Frame(m.palette, "Platform", body, m.width, m.height, theme.BorderDefault)
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
