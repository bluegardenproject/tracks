package tracksview

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/train"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// agentsState is Settings → Trains (agents): the agents Tracks starts
// beside a track's own, each in a frame of its own. The reviewer is the
// first.
type agentsState struct {
	agents  settings.Agents
	loadErr error
	focus   int // reviewerAgent, reviewerModel or reviewerPrefix
	hover   int // the field under the mouse, -1 for none
	// prefix edits the reviewer's prefix while editingPrefix;
	// prefixErr says why the last one typed was refused.
	prefix        textinput.Model
	editingPrefix bool
	prefixErr     string
	// saving is a save on its way; pending is another one due after it.
	saving, pending bool
}

// The reviewer frame's fields.
const (
	reviewerAgent = iota
	reviewerModel
	reviewerPrefix
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
// it. Its prefix stays.
func (m Model) setReviewer(d *settings.TrackType) (Model, tea.Cmd) {
	prefix := m.settings.agents.agents.ReviewerPrefix()
	switch {
	case d != nil:
		m.settings.agents.agents.Reviewer = &settings.Agent{Engine: d.Engine, Model: d.Model, Prefix: prefix}
	case prefix != "":
		m.settings.agents.agents.Reviewer = &settings.Agent{Prefix: prefix}
	default:
		m.settings.agents.agents.Reviewer = nil
	}
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

// setReviewerPrefix makes p the prefix of the reviewer's handles, ""
// for its name, and saves it.
func (m Model) setReviewerPrefix(p string) (Model, tea.Cmd) {
	r := m.settings.agents.agents.Reviewer
	switch {
	case r != nil:
		c := *r
		c.Prefix = p
		r = &c
	case p != "":
		r = &settings.Agent{Prefix: p}
	}
	if r != nil && *r == (settings.Agent{}) {
		r = nil
	}
	m.settings.agents.agents.Reviewer = r
	shown := p
	if shown == "" {
		shown = train.Reviewer.Prefix()
	}
	m.settings.notice = notice{text: "New reviewers take handles such as " + shown + "-527192."}
	return m.saveAgents()
}

// startPrefix starts editing the reviewer's prefix.
func (m Model) startPrefix() Model {
	a := &m.settings.agents
	a.focus, a.editingPrefix, a.prefixErr = reviewerPrefix, true, ""
	a.prefix = widget.NewInput(train.MaxPrefix, train.Reviewer.Prefix())
	a.prefix.SetValue(a.agents.ReviewerPrefix())
	a.prefix.CursorEnd()
	a.prefix.Focus()
	return m
}

// stopPrefix drops an edit of the prefix, as focus leaves it.
func (m Model) stopPrefix() Model {
	a := &m.settings.agents
	if a.editingPrefix {
		a.editingPrefix, a.prefixErr = false, ""
		a.prefix.Blur()
	}
	return m
}

// prefixKey handles a key while the prefix is edited: Enter keeps a
// prefix that can be one, empty for the reviewer's name; Esc drops the
// edit.
func (m Model) prefixKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	a := &m.settings.agents
	switch msg.String() {
	case "esc":
		return m.stopPrefix(), nil
	case "enter":
		p := strings.TrimSpace(a.prefix.Value())
		if p != "" {
			if err := train.CheckPrefix(p); err != nil {
				a.prefixErr = err.Error()
				return m, nil
			}
		}
		a.editingPrefix, a.prefixErr = false, ""
		a.prefix.Blur()
		return m.setReviewerPrefix(p)
	}
	return m.updatePrefix(msg)
}

// updatePrefix hands msg to the prefix's input; its error goes once its
// text changes.
func (m Model) updatePrefix(msg tea.Msg) (Model, tea.Cmd) {
	a := &m.settings.agents
	before := a.prefix.Value()
	var cmd tea.Cmd
	a.prefix, cmd = a.prefix.Update(msg)
	if a.prefix.Value() != before {
		a.prefixErr = ""
	}
	return m, cmd
}

// agentsPaste pastes into the prefix while it's edited: a paste of the
// terminal's, or of the input's own Ctrl+V.
func (m Model) agentsPaste(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.updatePrefix(msg)
}

// editingPrefix reports whether the prefix is edited now, with focus.
func (m Model) editingPrefix() bool {
	return m.tab == tabSettings && m.settings.section == sectionAgents && m.settings.editing && m.settings.agents.editingPrefix
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
		lines = append(lines, label("Agent")+m.selectField(sameAsTrack, fw, lit(reviewerAgent)),
			label("Model")+m.fg(theme.TextMuted).Render("The track's"))
		return append(lines, m.prefixLines(width, fw, lit(reviewerPrefix))...)
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
	lines = append(lines, m.prefixLines(width, fw, lit(reviewerPrefix))...)
	if m.engines.settings.Get(en.ID) == nil {
		warn := en.Name + " isn't added on the Engines tab, so reviews can't run."
		lines = append(lines, "")
		lines = append(lines, strings.Split(m.fg(theme.StateWarningText).Width(max(1, width)).Render(warn), "\n")...)
	}
	return lines
}

// prefixLines are the Prefix field, and why the last prefix typed was
// refused.
func (m Model) prefixLines(width, fw int, lit bool) []string {
	a := m.settings.agents
	bracket := theme.BorderDefault
	if lit {
		bracket = theme.BorderFocus
	}
	label := m.fg(theme.TextFaint).Render(pad("Prefix", labelWidth))
	var field string
	if a.editingPrefix {
		field = widget.Input(m.palette, a.prefix, fw, bracket)
	} else {
		value, tone := a.agents.ReviewerPrefix(), theme.TextDefault
		if value == "" {
			value, tone = train.Reviewer.Prefix()+" (its name)", theme.TextMuted
		}
		bg := m.fg(tone).Background(m.palette.Color(theme.InputBg))
		field = m.fg(bracket).Render("[") + bg.Render(pad(" "+cut(value, fw-2), fw)) + m.fg(bracket).Render("]")
	}
	lines := []string{label + field}
	if a.prefixErr != "" {
		lines = append(lines, strings.Split(m.fg(theme.StateDangerText).Width(max(1, width)).Render(a.prefixErr), "\n")...)
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
		"button shows. It only reads: it never edits, commits or pushes. Each review's handle on the relay is " +
		"the prefix and a number, such as reviewer-527192."
	return strings.Split(m.fg(theme.TextMuted).Width(max(1, width)).Render(about), "\n")
}

// agentAt is the reviewer field at bx, by inside the section, -1 for
// none.
func (m Model) agentAt(bx, by int) int {
	width := m.sectionWidth()
	// The intro, the frame's top border, its text and a blank line.
	field := by - (len(m.agentsIntro(width)) + 1 + len(m.reviewerAbout(max(1, width-4))) + 1)
	x := bx - 2 // the frame's border and padding
	if field < reviewerAgent || field > reviewerPrefix || x < labelWidth || x >= labelWidth+m.reviewerFieldWidth()+2 {
		return -1
	}
	if field == reviewerModel && m.reviewerFollows() {
		return -1
	}
	return field
}

func (m Model) agentsClick(bx, by int) (Model, tea.Cmd) {
	f := m.agentAt(bx, by)
	switch {
	case f == reviewerPrefix && m.settings.agents.editingPrefix:
		return m, nil
	case f == reviewerPrefix:
		return m.startPrefix(), nil
	case f >= 0:
		m = m.stopPrefix()
		m.settings.agents.focus = f
		return m.openReviewerPicker()
	}
	return m.stopPrefix(), nil
}

// agentsKey handles a key while the Trains (agents) section has focus.
// The model is skipped while the reviewer follows the track.
func (m Model) agentsKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	a := &m.settings.agents
	if a.editingPrefix {
		return m.prefixKey(msg)
	}
	step := 0
	switch msg.String() {
	case "up", "k", "shift+tab":
		step = -1
	case "down", "j", "tab":
		step = 1
	case "enter", "space":
		if a.focus == reviewerPrefix {
			return m.startPrefix(), nil
		}
		return m.openReviewerPicker()
	}
	a.focus = min(reviewerPrefix, max(reviewerAgent, a.focus+step))
	if a.focus == reviewerModel && m.reviewerFollows() {
		if step == 0 {
			step = 1
		}
		a.focus += step
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
	for i, en := range agents.Runners() {
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
		en := agents.Runners()[i-1]
		model := ""
		if !m.reviewerFollows() && en.ID == m.reviewerEngine().ID {
			model = m.reviewerModel()
		}
		return m.setReviewer(&settings.TrackType{Engine: en.ID, Model: model})
	}
	return m, nil
}
