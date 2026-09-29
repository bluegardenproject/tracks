package tracksview

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

// ResumeFunc starts ended track id again, telling progress each slow
// step, and switches to its window.
type ResumeFunc func(id string, progress func(string)) error

// CleanFunc removes ended track id's worktrees. Unless force, it
// returns the unsaved work it finds instead, one line per worktree.
type CleanFunc func(id string, force bool) (unsaved []string, err error)

type (
	// resumeEvent is a Resume's progress, or its end when done.
	resumeEvent struct {
		name     string
		progress string
		done     bool
		err      error
		events   <-chan resumeEvent
	}
	// checkedMsg is the unsaved work Clean's check found in track id,
	// cleanedMsg what Clean did.
	checkedMsg struct {
		id, name string
		unsaved  []string
		err      error
	}
	cleanedMsg checkedMsg
)

// startResume resumes t, showing its progress in the hint row.
func (m Model) startResume(t source.Track) (Model, tea.Cmd) {
	if m.resume == nil {
		return m, nil
	}
	events := make(chan resumeEvent, 16)
	resume := m.resume
	go func() {
		err := resume(t.ID, func(s string) { events <- resumeEvent{progress: s, events: events} })
		events <- resumeEvent{name: t.Name, done: true, err: err, events: events}
	}()
	m.station.notice = notice{text: "Resuming " + t.Name + "…"}
	return m, nextEvent(events)
}

func nextEvent(events <-chan resumeEvent) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func (m Model) resumed(e resumeEvent) (Model, tea.Cmd) {
	if !e.done {
		m.station.notice = notice{text: e.progress}
		return m, nextEvent(e.events)
	}
	if e.err != nil {
		m.station.notice = notice{failure("Couldn't resume "+e.name, e.err), true}
	} else {
		m.station.notice = notice{text: "Resumed " + e.name + "."}
	}
	return m, m.loadTracks(false)
}

// checkClean looks for unsaved work in t's worktrees, and then asks.
func (m Model) checkClean(t source.Track) (Model, tea.Cmd) {
	if m.unsaved == nil {
		return m, nil
	}
	unsaved := m.unsaved
	m.station.notice = notice{text: "Checking the worktrees of " + t.Name + "…"}
	return m, func() tea.Msg {
		found, err := unsaved(t.ID)
		return checkedMsg{t.ID, t.Name, found, err}
	}
}

func (m Model) checked(msg checkedMsg) Model {
	if msg.err != nil {
		m.station.notice = notice{failure("Couldn't check "+msg.name, msg.err), true}
		return m
	}
	m.station.notice = notice{}
	return m.ask(question{id: msg.id, name: msg.name, clean: true, unsaved: msg.unsaved})
}

// ask puts q to the user while its track is still the selected one.
func (m Model) ask(q question) Model {
	if t, ok := m.selectedTrack(); ok && t.ID == q.id {
		m.station.asking = &q
	}
	return m
}

// clean removes q's worktrees, anyway when q listed unsaved work.
func (m Model) clean(q question) tea.Cmd {
	if m.cleanFn == nil {
		return nil
	}
	clean := m.cleanFn
	return func() tea.Msg {
		found, err := clean(q.id, len(q.unsaved) > 0)
		return cleanedMsg{q.id, q.name, found, err}
	}
}

func (m Model) cleaned(msg cleanedMsg) (Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.station.notice = notice{failure("Couldn't clean "+msg.name, msg.err), true}
	case len(msg.unsaved) > 0:
		// Work that appeared after the check: ask again.
		m = m.ask(question{id: msg.id, name: msg.name, clean: true, unsaved: msg.unsaved})
	default:
		m.station.notice = notice{text: "Removed the worktrees of " + msg.name + "."}
	}
	return m, m.loadTracks(false)
}

// failure is err for the hint row: a problem is worded for the user
// already, anything else follows what failed.
func failure(what string, err error) string {
	var p tracks.Problem
	if errors.As(err, &p) {
		return p.Error()
	}
	return what + ": " + err.Error()
}
