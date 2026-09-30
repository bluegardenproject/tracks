package tracksview

import tea "charm.land/bubbletea/v2"

// promoteEvent is a Promote's progress, or its end when done.
type promoteEvent struct {
	name     string
	progress string
	done     bool
	err      error
	events   <-chan promoteEvent
}

// startPromote promotes track id, showing its progress in the hint row.
func (m Model) startPromote(id, name string) (Model, tea.Cmd) {
	if m.promote == nil {
		return m, nil
	}
	events := make(chan promoteEvent, 16)
	promote := m.promote
	go func() {
		err := promote(id, func(s string) { events <- promoteEvent{progress: s, events: events} })
		events <- promoteEvent{name: name, done: true, err: err, events: events}
	}()
	m.station.notice = notice{text: "Promoting " + name + "…", busy: true}
	return m, nextPromote(events)
}

func nextPromote(events <-chan promoteEvent) tea.Cmd {
	return func() tea.Msg { return <-events }
}

func (m Model) promoted(e promoteEvent) (Model, tea.Cmd) {
	switch {
	case !e.done:
		m.station.notice = notice{text: e.progress, busy: true}
		return m, nextPromote(e.events)
	case e.err != nil:
		m.station.notice = notice{text: failure("Couldn't promote "+e.name, e.err), err: true}
	default:
		m.station.notice = notice{text: "Promoted " + e.name + ": it has its own worktree now."}
	}
	return m, m.loadTracks()
}
