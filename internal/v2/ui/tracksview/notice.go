package tracksview

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// noticeFor is how long a notice that reports something done stays.
const noticeFor = 5 * time.Second

// noticeTick starts a notice's clock; tests stop it.
var noticeTick = tea.Tick

// notice is a short message in the hint row, until the next key, a
// click on its ✕ or, unless it's an error or busy, noticeFor.
type notice struct {
	text string
	err  bool
	busy bool // work under way: it stays until the work reports
}

// closable says n has a ✕.
func (n notice) closable() bool { return n.text != "" && !n.busy }

// noticeExpiredMsg ends the notice of tab that was its seq'th.
type noticeExpiredMsg struct{ tab, seq int }

// noticeTabs are the tabs with a notice.
var noticeTabs = []int{tabStation, tabRepositories, tabEngines, tabSettings}

func (m *Model) noticeOf(tab int) *notice {
	switch tab {
	case tabStation:
		return &m.station.notice
	case tabRepositories:
		return &m.repos.notice
	case tabEngines:
		return &m.engines.notice
	case tabSettings:
		return &m.settings.notice
	}
	return nil
}

func (m Model) notices() [tabSettings + 1]notice {
	var out [tabSettings + 1]notice
	for _, tab := range noticeTabs {
		out[tab] = *m.noticeOf(tab)
	}
	return out
}

// timeNotices starts the clock of each notice that changed since
// before and reports something done.
func (m Model) timeNotices(before [tabSettings + 1]notice) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	for _, tab := range noticeTabs {
		n := *m.noticeOf(tab)
		if n == before[tab] {
			continue
		}
		m.noticeSeq[tab]++
		if n.text != "" && !n.err && !n.busy {
			msg := noticeExpiredMsg{tab, m.noticeSeq[tab]}
			cmds = append(cmds, noticeTick(noticeFor, func(time.Time) tea.Msg { return msg }))
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) expire(msg noticeExpiredMsg) Model {
	if m.noticeSeq[msg.tab] == msg.seq {
		*m.noticeOf(msg.tab) = notice{}
	}
	return m
}

// noticeView is n for the hint row, with its ✕.
func (m Model) noticeView(n notice) string {
	color := theme.StateSuccessText
	if n.err {
		color = theme.StateDangerText
	}
	s := "  " + m.fg(color).Render(n.text)
	if n.closable() {
		s += "  " + m.fg(theme.TextMuted).Render("✕")
	}
	return s
}

// onNoticeClose reports whether x, y is the shown notice's ✕, give or
// take a cell.
func (m Model) onNoticeClose(x, y int) bool {
	n := m.noticeOf(m.tab)
	if n == nil || !n.closable() || y != m.height-1 {
		return false
	}
	at := 2 + lipgloss.Width(n.text) + 2
	return x >= at-1 && x <= at+1
}
