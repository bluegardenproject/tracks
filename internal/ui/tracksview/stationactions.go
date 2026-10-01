package tracksview

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

// ResumeFunc starts ended track id again, telling progress each slow
// step, and switches to its window. When some of its worktrees are gone
// it returns them instead, one line per worktree, unless recreate.
type ResumeFunc func(id string, recreate bool, progress func(string)) (missing []string, err error)

// DiscardFunc archives or derails ended track id, removing its
// worktrees and branches. Unless force, it returns the work that would
// be lost instead, one line per repo.
type DiscardFunc func(id string, force bool) (lost []string, err error)

type (
	// resumeEvent is a Resume's progress, or its end when done.
	resumeEvent struct {
		id, name string
		progress string
		done     bool
		missing  []string
		err      error
		events   <-chan resumeEvent
	}
	// checkedMsg is the work archiving or derailing track id would
	// lose, found before asking kind; archivedMsg is what Archive did,
	// derailedMsg what Derail did.
	checkedMsg struct {
		id, name string
		kind     asked
		lost     []string
		err      error
	}
	archivedMsg checkedMsg
	derailedMsg checkedMsg
)

// startResume resumes track id, showing its progress in the hint row.
func (m Model) startResume(id, name string, recreate bool) (Model, tea.Cmd) {
	if m.resume == nil {
		return m, nil
	}
	events := make(chan resumeEvent, 16)
	resume := m.resume
	go func() {
		missing, err := resume(id, recreate, func(s string) { events <- resumeEvent{progress: s, events: events} })
		events <- resumeEvent{id: id, name: name, done: true, missing: missing, err: err, events: events}
	}()
	m.station.notice = notice{text: "Resuming " + name + "…", busy: true}
	return m, nextEvent(events)
}

func nextEvent(events <-chan resumeEvent) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func (m Model) resumed(e resumeEvent) (Model, tea.Cmd) {
	if !e.done {
		m.station.notice = notice{text: e.progress, busy: true}
		return m, nextEvent(e.events)
	}
	switch {
	case e.err != nil:
		m.station.notice = notice{text: failure("Couldn't resume "+e.name, e.err), err: true}
	case len(e.missing) > 0:
		m.station.notice = notice{}
		if m = m.ask(question{id: e.id, name: e.name, kind: askRecreate, lines: e.missing}); m.station.asking == nil {
			m.station.notice = notice{text: "Couldn't resume " + e.name + ": a worktree couldn't be found.", err: true}
		}
	default:
		m.station.notice = notice{text: "Resumed " + e.name + "."}
	}
	return m, m.loadTracks()
}

// checkLost looks for the work doing kind to t would lose, saying so
// with checking, and then asks kind.
func (m Model) checkLost(t source.Track, kind asked, checking string) (Model, tea.Cmd) {
	if m.lostFn == nil {
		return m, nil
	}
	lost := m.lostFn
	m.station.notice = notice{text: checking, busy: true}
	return m, func() tea.Msg {
		found, err := lost(t.ID)
		return checkedMsg{id: t.ID, name: t.Name, kind: kind, lost: found, err: err}
	}
}

func (m Model) checked(msg checkedMsg) Model {
	if msg.err != nil {
		m.station.notice = notice{text: failure("Couldn't check "+msg.name, msg.err), err: true}
		return m
	}
	m.station.notice = notice{}
	return m.ask(question{id: msg.id, name: msg.name, kind: msg.kind, lines: msg.lost})
}

// ask puts q to the user while its track is still the selected one.
func (m Model) ask(q question) Model {
	if t, ok := m.selectedTrack(); ok && t.ID == q.id {
		m.station.asking = &q
	}
	return m
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
