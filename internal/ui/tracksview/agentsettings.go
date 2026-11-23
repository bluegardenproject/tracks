package tracksview

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// agentsState is Settings → Trains (agents): the agents Tracks starts
// beside a track's own, each in a frame of its own. The reviewer is the
// first.
type agentsState struct {
	agents  settings.Agents
	loadErr error
	focus   int // reviewerAgent or reviewerModel
	hover   int // the field under the mouse, -1 for none
	// saving is a save on its way; pending is another one due after it.
	saving, pending bool
}

// The reviewer frame's fields.
const (
	reviewerAgent = iota
	reviewerModel
)

// sameAsTrack is the reviewer's agent when it follows the reviewed
// track's.
const sameAsTrack = "Same as the track"

type (
	agentsMsg struct {
		agents settings.Agents
		err    error
	}
	agentsSavedMsg struct{ err error }
)

func (m Model) loadAgents() tea.Cmd {
	if m.agentSource == nil {
		return nil
	}
	src := m.agentSource
	return func() tea.Msg {
		a, err := src.Load()
		return agentsMsg{a, err}
	}
}

func (m Model) setAgents(msg agentsMsg) Model {
	a := &m.settings.agents
	if !a.saving && !a.pending {
		a.agents = msg.agents
	}
	a.loadErr = msg.err
	return m
}

// setReviewer makes d the reviewer, nil to follow the track, and saves
// it.
func (m Model) setReviewer(d *settings.TrackType) (Model, tea.Cmd) {
	m.settings.agents.agents.Reviewer = d
	text := "Reviews run on the reviewed track's own agent and model."
	if d != nil {
		en, _ := agents.ByID(d.Engine)
		text = "Reviews run on " + en.Name
		if d.Model != "" {
			text += ", model " + d.Model
		}
		text += "."
	}
	m.settings.notice = notice{text: text}
	return m.saveAgents()
}

func (m Model) saveAgents() (Model, tea.Cmd) {
	a := &m.settings.agents
	if m.agentSource == nil {
		return m, nil
	}
	if a.saving {
		a.pending = true
		return m, nil
	}
	a.saving = true
	src, saved := m.agentSource, a.agents
	return m, func() tea.Msg { return agentsSavedMsg{src.Save(saved)} }
}

func (m Model) agentsSaved(msg agentsSavedMsg) (Model, tea.Cmd) {
	a := &m.settings.agents
	a.saving = false
	if msg.err != nil {
		m.settings.notice = notice{text: "Couldn't save the agents: " + msg.err.Error(), err: true}
	}
	if a.pending {
		a.pending = false
		return m.saveAgents()
	}
	return m, nil
}

// reviewerFollows reports whether the reviewer runs on the reviewed
// track's own agent and model.
func (m Model) reviewerFollows() bool {
	d := m.settings.agents.agents.Reviewer
	return d == nil || d.Engine == ""
}

// reviewerEngine is the reviewer's own agent; only meaningful when it
// doesn't follow the track. An agent this build doesn't know, from a
// hand-edited file, keeps its ID as its name.
func (m Model) reviewerEngine() agents.Engine {
	d := m.settings.agents.agents.Reviewer
	if d == nil {
		return agents.Claude
	}
	if en, ok := agents.ByID(d.Engine); ok {
		return en
	}
	return agents.Engine{ID: d.Engine, Name: d.Engine}
}

// reviewerModel is the reviewer's own model, "" for its agent's default.
func (m Model) reviewerModel() string {
	if d := m.settings.agents.agents.Reviewer; d != nil {
		return d.Model
	}
	return ""
}

// agentsIntro is the section's text above the frames.
func (m Model) agentsIntro(width int) []string {
	about := "Each runs in a fresh session of its own, on the agent and model you pick here."
	lines := []string{m.fg(theme.TextDefault).Render("Agents beside a track's own")}
	lines = append(lines, strings.Split(m.fg(theme.TextMuted).Width(max(1, width)).Render(about), "\n")...)
	return append(lines, "")
}

// reviewerBody is the Reviewer frame's inside.
func (m Model) reviewerBody(width int) []string {
	a := m.settings.agents
	lit := func(field int) bool { return a.hover == field || m.settings.editing && a.focus == field }
	fw := m.reviewerFieldWidth()
	label := func(s string) string { return m.fg(theme.TextFaint).Render(pad(s, labelWidth)) }
	lines := append(m.reviewerAbout(width), "")
	if m.reviewerFollows() {
		return append(lines, label("Agent")+m.selectField(sameAsTrack, fw, lit(reviewerAgent)),
			label("Model")+m.fg(theme.TextMuted).Render("The track's"))
	}
	en := m.reviewerEngine()
	text := m.reviewerModel()
	if text == "" {
		text = "Default"
		if conf := m.engines.settings.Get(en.ID); conf != nil && conf.Model != "" {
			text = "Default (" + conf.Model + ")"
		}
	}
	model := label("Model") + m.selectField(text, fw, lit(reviewerModel))
	if name := m.reviewerModel(); name != "" && !m.listed(en, name) {
		model += m.fg(theme.StateWarningText).Render(cut("  "+notListed, max(0, width-labelWidth-fw-2)))
	}
	lines = append(lines, label("Agent")+m.selectField(en.Name, fw, lit(reviewerAgent)), model)
	if m.engines.settings.Get(en.ID) == nil {
		warn := en.Name + " isn't added on the Engines tab, so reviews can't run."
		lines = append(lines, "")
		lines = append(lines, strings.Split(m.fg(theme.StateWarningText).Width(max(1, width)).Render(warn), "\n")...)
	}
	return lines
}

func (m Model) agentsView(width int) []string {
	body := m.reviewerBody(max(1, width-4))
	lines := append(m.agentsIntro(width), widget.Frame(m.palette, "Reviewer", body, width, len(body)+2, theme.BorderDefault)...)
	if err := m.settings.agents.loadErr; err != nil {
		lines = append(lines, "", m.fg(theme.StateDangerText).Render(cut("Couldn't read the settings: "+err.Error(), width)))
	}
	return lines
}

// reviewerFieldWidth is the inside width of the Reviewer frame's
// fields.
func (m Model) reviewerFieldWidth() int {
	return min(m.themeFieldWidth(), max(4, m.sectionWidth()-4-labelWidth-2))
}

// reviewerAbout is the Reviewer frame's text above its fields.
func (m Model) reviewerAbout(width int) []string {
	about := "Reviews a track's changes when its agent runs `tracks review`, in a pane of the track the footer's " +
		"button shows. It only reads: it never edits, commits or pushes."
	return strings.Split(m.fg(theme.TextMuted).Width(max(1, width)).Render(about), "\n")
}

// agentAt is the reviewer field at bx, by inside the section, -1 for
// none.
func (m Model) agentAt(bx, by int) int {
	width := m.sectionWidth()
	// The intro, the frame's top border, its text and a blank line.
	field := by - (len(m.agentsIntro(width)) + 1 + len(m.reviewerAbout(max(1, width-4))) + 1)
	x := bx - 2 // the frame's border and padding
	if field < reviewerAgent || field > reviewerModel || x < labelWidth || x >= labelWidth+m.reviewerFieldWidth()+2 {
		return -1
	}
	if field == reviewerModel && m.reviewerFollows() {
		return -1
	}
	return field
}

func (m Model) agentsClick(bx, by int) (Model, tea.Cmd) {
	if f := m.agentAt(bx, by); f >= 0 {
		m.settings.agents.focus = f
		return m.openReviewerPicker()
	}
	return m, nil
}

// agentsKey handles a key while the Trains (agents) section has focus.
func (m Model) agentsKey(key string) (Model, tea.Cmd) {
	a := &m.settings.agents
	last := reviewerModel
	if m.reviewerFollows() {
		last = reviewerAgent
	}
	a.focus = min(a.focus, last)
	switch key {
	case "up", "k", "shift+tab":
		a.focus = max(reviewerAgent, a.focus-1)
	case "down", "j", "tab":
		a.focus = min(last, a.focus+1)
	case "enter", "space":
		return m.openReviewerPicker()
	}
	return m, nil
}

// openReviewerPicker opens the picker for the focused reviewer field.
func (m Model) openReviewerPicker() (Model, tea.Cmd) {
	if m.settings.agents.focus == reviewerModel {
		if m.reviewerFollows() {
			return m, nil
		}
		return m.modelPicker(m.reviewerEngine(), pickReviewerModel, "Reviewer: model")
	}
	items := []widget.PickerItem{{Label: sameAsTrack, Detail: "the reviewed track's agent and model"}}
	marked := 0
	for i, en := range agents.All {
		item := widget.PickerItem{Label: en.Name}
		if m.engines.settings.Get(en.ID) == nil {
			item.Detail = "not added"
		}
		if !m.reviewerFollows() && en.ID == m.reviewerEngine().ID {
			marked = i + 1
		}
		items = append(items, item)
	}
	p := widget.NewPicker("Reviewer: agent", items, marked)
	p.Cursor = marked
	m.picker, m.pickerFor = &p, pickReviewerAgent
	return m, nil
}

// reviewerAgentPicked acts on what the reviewer's agent picker did.
// Another agent starts on its default model.
func (m Model) reviewerAgentPicked(r widget.PickerResult) (Model, tea.Cmd) {
	switch r {
	case widget.PickerClosed:
		m.picker = nil
	case widget.PickerChosen:
		i := m.picker.Cursor
		m.picker = nil
		if i == 0 {
			return m.setReviewer(nil)
		}
		en := agents.All[i-1]
		model := ""
		if !m.reviewerFollows() && en.ID == m.reviewerEngine().ID {
			model = m.reviewerModel()
		}
		return m.setReviewer(&settings.TrackType{Engine: en.ID, Model: model})
	}
	return m, nil
}
