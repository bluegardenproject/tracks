package tracksview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/theme"
)

// notifyState is Settings → General → Notifications: the channels and
// the events that send one.
type notifyState struct {
	notify  settings.Notifications
	loadErr error
	// saving is a save on its way; pending is another one due after it.
	saving, pending bool
}

// notifyRows are the toggles, the channels first. General's focus is
// the theme field (0), then a row each.
var notifyRows = []struct{ key, label string }{
	{settings.NotifyMacOS, "macOS notifications"},
	{settings.NotifyBell, "Terminal bell"},
	{settings.NotifyActionRequired, "A track needs you"},
	{settings.NotifyError, "A track's agent fails"},
	{settings.NotifyPROpened, "A track opens a PR"},
	{settings.NotifyPRSettled, "A track's PR is merged or closed"},
}

const notifyChannels = 2

type (
	notifyMsg struct {
		notify settings.Notifications
		err    error
	}
	notifySavedMsg struct{ err error }
)

func (m Model) loadNotify() tea.Cmd {
	if m.notifySource == nil {
		return nil
	}
	src := m.notifySource
	return func() tea.Msg {
		n, err := src.Load()
		return notifyMsg{n, err}
	}
}

func (m Model) setNotify(msg notifyMsg) Model {
	n := &m.settings.notify
	if !n.saving && !n.pending {
		n.notify = msg.notify
	}
	n.loadErr = msg.err
	return m
}

func (m Model) saveNotify() (Model, tea.Cmd) {
	n := &m.settings.notify
	if m.notifySource == nil {
		return m, nil
	}
	if n.saving {
		n.pending = true
		return m, nil
	}
	n.saving = true
	src, notify := m.notifySource, n.notify
	return m, func() tea.Msg { return notifySavedMsg{src.Save(notify)} }
}

func (m Model) notifySaved(msg notifySavedMsg) (Model, tea.Cmd) {
	n := &m.settings.notify
	n.saving = false
	if msg.err != nil {
		m.settings.notice = notice{text: "Couldn't save the notifications: " + msg.err.Error(), err: true}
	}
	if n.pending {
		n.pending = false
		return m.saveNotify()
	}
	return m, nil
}

// toggleNotify switches row i on or off and saves it.
func (m Model) toggleNotify(i int) (Model, tea.Cmd) {
	n := &m.settings.notify
	r := notifyRows[i]
	on := !n.notify.On(r.key)
	n.notify = n.notify.Set(r.key, on)
	text := r.label + " off."
	switch {
	case i < notifyChannels && on:
		text = r.label + " on."
	case i >= notifyChannels:
		what := strings.ToLower(r.label[:1]) + r.label[1:]
		text = "Not notifying when " + what + "."
		if on {
			text = "Notifying when " + what + "."
		}
	}
	m.settings.notice = notice{text: text}
	return m.saveNotify()
}

// pressGeneral acts on General's focused field: the theme picker
// opens, a toggle switches.
func (m Model) pressGeneral() (Model, tea.Cmd) {
	if f := m.settings.generalFocus; f > 0 {
		return m.toggleNotify(f - 1)
	}
	return m.openPicker(pickUse)
}

// generalKey handles a key while General has focus.
func (m Model) generalKey(key string) (Model, tea.Cmd) {
	s := &m.settings
	switch key {
	case "up", "k", "shift+tab":
		s.generalFocus = max(0, s.generalFocus-1)
	case "down", "j", "tab":
		s.generalFocus = min(len(notifyRows), s.generalFocus+1)
	case "enter", "space":
		return m.pressGeneral()
	}
	return m, nil
}

// notifyView is the Notifications group, and the lines its rows are
// on, counted from its top.
func (m Model) notifyView(width int) (lines []string, rows []int) {
	s := m.settings
	wrap := func(c theme.Token, text string) []string {
		return strings.Split(m.fg(c).Width(max(1, width)).Render(text), "\n")
	}
	lines = []string{m.fg(theme.TextDefault).Render("Notifications")}
	lines = append(lines, wrap(theme.TextMuted, "For tracks whose window isn't on screen. The bell rings in the track's window, and your terminal hears it.")...)
	for i, r := range notifyRows {
		switch i {
		case 0:
			lines = append(lines, "", m.fg(theme.TextFaint).Render("Send as"))
		case notifyChannels:
			lines = append(lines, "", m.fg(theme.TextFaint).Render("Notify when"))
		}
		box, color := "[ ]", theme.BorderDefault
		if s.notify.notify.On(r.key) {
			box = "[x]"
		}
		if s.generalHover == i+1 || s.editing && s.generalFocus == i+1 {
			color = theme.BorderFocus
		}
		rows = append(rows, len(lines))
		lines = append(lines, m.fg(color).Render(box)+" "+m.fg(theme.TextDefault).Render(cut(r.label, max(0, width-4))))
	}
	if s.notify.loadErr != nil {
		lines = append(lines, "", m.fg(theme.StateDangerText).Render(cut("Couldn't read the settings: "+s.notify.loadErr.Error(), width)))
	}
	return lines, rows
}

// notifyTop is the Notifications group's first line in General.
func (m Model) notifyTop() int {
	n := len(m.generalIntro(m.sectionWidth())) + 1
	if m.settings.themesErr != nil {
		n += 2
	}
	return n + 1
}

// generalAt is General's field at x, y relative to the section's inside:
// 1 and on for the toggles, -1 for none. The theme field has its own
// test, onThemeField.
func (m Model) generalAt(bx, by int) int {
	width := m.sectionWidth()
	_, rows := m.notifyView(width)
	for i, row := range rows {
		if by == m.notifyTop()+row && bx >= 0 && bx < 4+len(notifyRows[i].label) {
			return i + 1
		}
	}
	return -1
}
