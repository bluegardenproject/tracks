package tracksview

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"
)

// reconnectEvery is how long Station waits to watch the tracks again
// once the daemon's change stream broke.
const reconnectEvery = time.Second

// watchMsg says the tracks changed, or the stream just connected; live
// is false once it broke.
type watchMsg struct{ live bool }

// watch follows the daemon's change stream for as long as the window
// runs, connecting again after it breaks. Its command never returns.
func (m Model) watch() tea.Cmd {
	if m.watchFn == nil {
		return nil
	}
	fn, changes := m.watchFn, m.changes
	return func() tea.Msg {
		for {
			_ = fn(context.Background(), func() { changes <- watchMsg{live: true} })
			changes <- watchMsg{}
			time.Sleep(reconnectEvery)
		}
	}
}

// nextChange waits for the stream's next message.
func (m Model) nextChange() tea.Cmd {
	if m.changes == nil {
		return nil
	}
	changes := m.changes
	return func() tea.Msg { return <-changes }
}

// watched reads the tracks again after a change, and notes a broken
// stream.
func (m Model) watched(msg watchMsg) (Model, tea.Cmd) {
	m.station.offline = !msg.live
	if !msg.live {
		return m, m.nextChange()
	}
	return m, tea.Batch(m.loadTracks(), m.nextChange())
}
