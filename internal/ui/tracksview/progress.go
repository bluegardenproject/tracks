package tracksview

import tea "charm.land/bubbletea/v2"

// progressEvent is an action's progress, or its end when done.
type progressEvent struct {
	progress string
	done     bool
	ok       string // the notice when it worked
	failed   string // what failed, before the error
	err      error
	events   <-chan progressEvent
}

// startProgress runs an action on a track, showing busy, then each step
// it tells, then ok or what failed in the hint row.
func (m Model) startProgress(busy, ok, failed string, run func(progress func(string)) error) (Model, tea.Cmd) {
	events := make(chan progressEvent, 16)
	go func() {
		err := run(func(s string) { events <- progressEvent{progress: s, events: events} })
		events <- progressEvent{done: true, ok: ok, failed: failed, err: err, events: events}
	}()
	m.station.notice = notice{text: busy, busy: true}
	return m, nextProgress(events)
}

func nextProgress(events <-chan progressEvent) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func (m Model) progressed(e progressEvent) (Model, tea.Cmd) {
	switch {
	case !e.done:
		m.station.notice = notice{text: e.progress, busy: true}
		return m, nextProgress(e.events)
	case e.err != nil:
		m.station.notice = notice{text: failure(e.failed, e.err), err: true}
	default:
		m.station.notice = notice{text: e.ok}
	}
	return m, m.loadTracks()
}

// startPromote promotes track id, name, to a Work track.
func (m Model) startPromote(id, name string) (Model, tea.Cmd) {
	if m.promote == nil {
		return m, nil
	}
	promote := m.promote
	return m.startProgress("Promoting "+name+"…", "Promoted "+name+": it has its own worktree now.", "Couldn't promote "+name,
		func(progress func(string)) error { return promote(id, progress) })
}

// startRestart starts track id's agent again.
func (m Model) startRestart(id, name string) (Model, tea.Cmd) {
	if m.restart == nil {
		return m, nil
	}
	restart := m.restart
	return m.startProgress("Restarting "+name+"…", "Restarted "+name+".", "Couldn't restart "+name,
		func(progress func(string)) error { return restart(id, progress) })
}
