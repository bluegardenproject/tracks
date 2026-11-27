// Package platform is the Platform: the pane on a track window's left
// that lists the track's trains, each with the Station's status badges,
// its side tracks, or a side track's way back to its main track,
// and under Signals its Relay Station and its PRs with open comments
// (see docs/design/trains.md). It reads them from the daemon, again on
// each change the daemon tells of. A click on a train opens its overlay.
package platform

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/style"
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

// Side is a side track as the Platform shows it: its name and the
// Station's badges, its status and its PRs'; ID names it for OpenSide.
// Main marks a side track's main track instead, its ID and name alone:
// the Platform shows a way back to it.
type Side struct {
	ID, Name   string
	Status, PR track.Status
	Main       bool
}

// PR is a PR with open comments, under Signals: its name, what its
// button does, and whether that's done; Index names it for Solve.
type PR struct {
	Index      int
	PR, Action string
	Sent       bool
}

// Signals are the Relay Station's count of messages the user hasn't
// seen, whether @github's events wait for a held pointer, and the PRs
// with open comments.
type Signals struct {
	RelayNew int
	Held     bool
	PRs      []PR
}

// Config is what the Platform reads, what its clicks do, and how.
type Config struct {
	Theme theme.Theme
	// Trains lists the track's trains, Sides its side tracks; Signals
	// reads its Signals.
	Trains  func() ([]Entry, error)
	Sides   func() ([]Side, error)
	Signals func() (Signals, error)
	// Watch calls changed once connected to the daemon and after each
	// change, until the stream breaks.
	Watch func(ctx context.Context, changed func()) error
	// LoadTheme reads the theme the user chose, now.
	LoadTheme func() (theme.Theme, error)
	// Collect sends the track's agent the pointer to its waiting
	// messages now; OpenRelayStation opens the Relay Station over the
	// user's client; Solve sends the agent the prompt to work through a
	// PR's comments.
	Collect          func() error
	OpenRelayStation func() error
	Solve            func(index int) error
	// Open opens a train, by its handle, in its overlay over the user's
	// client; OpenSide switches the user's client to a side track's
	// window, or a side track's main track's, by its ID.
	Open     func(handle string)
	OpenSide func(id string) error
}

// Model is the Platform.
type Model struct {
	palette       style.Palette
	trains        []Entry
	sides         []Side
	signals       Signals
	err           error
	width, height int
	c             Config
	changes       chan struct{}
	// notice says what a click did, or why it couldn't, and noticeFor
	// which kind of target it was; acting is a click's call on its way,
	// which other clicks wait for.
	notice    string
	noticeFor int
	acting    bool
}

type (
	loadedMsg struct {
		trains  []Entry
		sides   []Side
		signals Signals
		err     error
	}
	changedMsg struct{}
	themeMsg   struct {
		theme theme.Theme
		err   error
	}
	actedMsg struct{ err error }
)

// New is the Platform for c.
func New(c Config) Model {
	m := Model{palette: style.New(c.Theme), c: c}
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
	case loadedMsg:
		m.trains, m.sides, m.signals, m.err = msg.trains, msg.sides, msg.signals, msg.err
		if !m.acting && m.targetGone(m.noticeFor) {
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
		if msg.Button != tea.MouseLeft || m.acting {
			break
		}
		if act, kind, notice := m.actionAt(msg.X, msg.Y); act != nil {
			m.notice, m.noticeFor, m.acting = notice, kind, true
			return m, func() tea.Msg { return actedMsg{act()} }
		}
	case actedMsg:
		m.notice, m.acting = "", false
		if msg.err != nil {
			m.notice = msg.err.Error()
		}
		return m, m.load()
	}
	// No key quits it: the daemon keeps it open, and would only open
	// it again.
	return m, nil
}

// actionAt is what a click at x, y does, the kind of target it's on,
// and what to say meanwhile; nil for nothing.
func (m Model) actionAt(x, y int) (func() error, int, string) {
	_, targets := m.layout()
	for _, t := range targets {
		if y != t.y || x < t.x0 || x >= t.x1 {
			continue
		}
		switch t.kind {
		case targetCollect, targetCollectSignals:
			if m.c.Collect != nil {
				return m.c.Collect, t.kind, "Collecting…"
			}
		case targetRelayStation:
			if m.c.OpenRelayStation != nil {
				return m.c.OpenRelayStation, t.kind, ""
			}
		case targetSolve:
			if m.c.Solve != nil {
				index, solve := t.index, m.c.Solve
				return func() error { return solve(index) }, t.kind, "Sending…"
			}
		case targetSide:
			if m.c.OpenSide != nil && t.index < len(m.sides) {
				id, open := m.sides[t.index].ID, m.c.OpenSide
				return func() error { return open(id) }, t.kind, ""
			}
		case targetTrain:
			if m.c.Open != nil && t.index < len(m.trains) {
				handle, open := m.trains[t.index].Handle, m.c.Open
				return func() error { open(handle); return nil }, t.kind, ""
			}
		}
	}
	return nil, 0, ""
}

// targetGone reports whether no target of kind is shown any more: a
// notice about one goes with it. The Relay Station is always there, so
// what it said stays until the next click.
func (m Model) targetGone(kind int) bool {
	switch kind {
	case targetCollect:
		return !m.anyHeld()
	case targetCollectSignals:
		return !m.signals.Held
	case targetSide:
		return len(m.sides) == 0
	case targetSolve:
		for _, pr := range m.signals.PRs {
			if !pr.Sent {
				return false
			}
		}
		return true
	}
	return false
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

// nextTheme reads the theme again after themeEvery.
func (m Model) nextTheme() tea.Cmd {
	if m.c.LoadTheme == nil {
		return nil
	}
	load := m.c.LoadTheme
	return tea.Tick(themeEvery, func(time.Time) tea.Msg {
		t, err := load()
		return themeMsg{t, err}
	})
}

func (m Model) View() tea.View {
	lines, _ := m.layout()
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// load reads the trains and the Signals.
func (m Model) load() tea.Cmd {
	if m.c.Trains == nil {
		return nil
	}
	trains, sides, signals := m.c.Trains, m.c.Sides, m.c.Signals
	return func() tea.Msg {
		ts, err := trains()
		var ss []Side
		if err == nil && sides != nil {
			ss, err = sides()
		}
		var sig Signals
		if err == nil && signals != nil {
			sig, err = signals()
		}
		return loadedMsg{ts, ss, sig, err}
	}
}

// watch follows the daemon's changes, connecting again when the stream
// breaks.
func (m Model) watch() tea.Cmd {
	if m.c.Watch == nil {
		return nil
	}
	fn, changes := m.c.Watch, m.changes
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
