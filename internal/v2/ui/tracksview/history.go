package tracksview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// historyState is Settings → Tracks → Tracks History: whether old
// tracks are archived on their own.
type historyState struct {
	history settings.History
	loadErr error
	// saving is a save on its way; pending is another one due after it.
	saving, pending bool
}

// unsavedChoices are the picker's rows for Tracks with unsaved work.
var unsavedChoices = []struct{ value, label string }{
	{settings.UnsavedSkip, "Skip them"},
	{settings.UnsavedKeep, "Archive, keep the worktrees"},
}

type (
	historyMsg struct {
		history settings.History
		err     error
	}
	historySavedMsg struct{ err error }
)

func (m Model) loadHistory() tea.Cmd {
	if m.historySource == nil {
		return nil
	}
	src := m.historySource
	return func() tea.Msg {
		h, err := src.Load()
		return historyMsg{h, err}
	}
}

func (m Model) setHistory(msg historyMsg) Model {
	h := &m.settings.history
	if !h.saving && !h.pending {
		h.history = msg.history
	}
	h.loadErr = msg.err
	return m
}

func (m Model) saveHistory() (Model, tea.Cmd) {
	h := &m.settings.history
	if m.historySource == nil {
		return m, nil
	}
	if h.saving {
		h.pending = true
		return m, nil
	}
	h.saving = true
	src, history := m.historySource, h.history
	return m, func() tea.Msg { return historySavedMsg{src.Save(history)} }
}

func (m Model) historySaved(msg historySavedMsg) (Model, tea.Cmd) {
	h := &m.settings.history
	h.saving = false
	if msg.err != nil {
		m.settings.notice = notice{text: "Couldn't save Tracks History: " + msg.err.Error(), err: true}
	}
	if h.pending {
		h.pending = false
		return m.saveHistory()
	}
	return m, nil
}

// lastTypeField is the Tracks section's last control: Tracks with
// unsaved work shows only while auto-archive is on.
func (m Model) lastTypeField() int {
	if m.settings.history.history.AutoArchive {
		return typeUnsaved
	}
	return typeAutoArchive
}

// toggleAutoArchive turns auto-archive on or off and saves it.
func (m Model) toggleAutoArchive() (Model, tea.Cmd) {
	h := &m.settings.history.history
	h.AutoArchive = !h.AutoArchive
	text := "Tracks are archived a week after they end."
	if !h.AutoArchive {
		text = "Tracks stay in Station until you archive them."
	}
	m.settings.notice = notice{text: text}
	return m.saveHistory()
}

func (m Model) unsavedIndex() int {
	if m.settings.history.history.KeepUnsaved() {
		return 1
	}
	return 0
}

func (m Model) openUnsavedPicker() (Model, tea.Cmd) {
	items := make([]widget.PickerItem, len(unsavedChoices))
	for i, c := range unsavedChoices {
		items[i] = widget.PickerItem{Label: c.label}
	}
	p := widget.NewPicker("Tracks with unsaved work", items, m.unsavedIndex())
	p.Cursor = m.unsavedIndex()
	m.picker, m.pickerFor = &p, pickUnsaved
	return m, nil
}

func (m Model) unsavedPicked(r widget.PickerResult) (Model, tea.Cmd) {
	switch r {
	case widget.PickerClosed:
		m.picker = nil
	case widget.PickerChosen:
		choice := unsavedChoices[m.picker.Cursor]
		m.picker = nil
		m.settings.history.history.Unsaved = choice.value
		m.settings.notice = notice{text: "Tracks with unsaved work: " + strings.ToLower(choice.label[:1]) + choice.label[1:] + "."}
		return m.saveHistory()
	}
	return m, nil
}

// historyView is the Tracks History group, and the rows its controls
// are on, counted from its top.
func (m Model) historyView(width int) (lines []string, rows map[int]int) {
	h, t := m.settings.history, m.settings.types
	lit := func(field int) bool {
		return t.hover.field == field || m.settings.editing && t.focus == field
	}
	wrap := func(c theme.Token, s string) []string {
		return strings.Split(m.fg(c).Width(max(1, width)).Render(s), "\n")
	}
	rows = map[int]int{}
	lines = []string{m.fg(theme.TextDefault).Render("Tracks History")}
	lines = append(lines, wrap(theme.TextMuted, "Archived tracks leave Station; their branches stay.")...)
	lines = append(lines, "")

	box, color := "[ ]", theme.BorderDefault
	if h.history.AutoArchive {
		box = "[x]"
	}
	if lit(typeAutoArchive) {
		color = theme.BorderFocus
	}
	rows[typeAutoArchive] = len(lines)
	lines = append(lines, m.fg(color).Render(box)+" "+m.fg(theme.TextDefault).Render("Auto-archive tracks"))
	lines = append(lines, wrap(theme.TextMuted, "Tracks that ended more than 7 days ago, with no PR still open or in draft.")...)

	if h.history.AutoArchive {
		lines = append(lines, "", m.fg(theme.TextDefault).Render("Tracks with unsaved work"))
		rows[typeUnsaved] = len(lines)
		lines = append(lines, m.selectField(unsavedChoices[m.unsavedIndex()].label, m.themeFieldWidth(), lit(typeUnsaved)))
	}
	if h.loadErr != nil {
		lines = append(lines, "", m.fg(theme.StateDangerText).Render(cut("Couldn't read the settings: "+h.loadErr.Error(), width)))
	}
	return lines, rows
}
