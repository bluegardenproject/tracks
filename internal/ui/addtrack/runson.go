package addtrack

import (
	"context"
	"maps"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// Engine is an engine added on the Engines tab, as the form offers it.
type Engine struct {
	ID, Name string
	// Default is its default model on the Engines tab, "" for its own.
	Default string
	// Models are its models, Added the ones the user added. One that
	// Lists its own gets them from Config.Models instead.
	Models []agents.Model
	Added  []string
	Lists  bool
}

// ModelsFunc lists the models of the engine with id.
type ModelsFunc func(ctx context.Context, id string) ([]agents.Model, error)

// RunsOn is what a type's tracks run on: the engine's ID, "" with none
// added, and the type's own model, "" for the engine's default.
type RunsOn struct{ Engine, Model string }

// listing is an engine's own model list, while and once it's asked.
type listing struct {
	models []agents.Model
	err    error
	done   bool
}

type modelsMsg struct {
	id     string
	models []agents.Model
	err    error
}

const notListed = "not listed"

// asking marks every engine that lists its own models as asked, which
// Init then does.
func asking(engines []Engine, list ModelsFunc) map[string]listing {
	out := map[string]listing{}
	for _, e := range engines {
		if e.Lists && list != nil {
			out[e.ID] = listing{}
		}
	}
	return out
}

// listCmds ask for the models of the engines asked and not answered.
func (m Model) listCmds() tea.Cmd {
	var cmds []tea.Cmd
	for _, e := range m.engines {
		if l, asked := m.listed[e.ID]; asked && !l.done {
			cmds = append(cmds, m.listCmd(e.ID))
		}
	}
	return tea.Batch(cmds...)
}

func (m Model) listCmd(id string) tea.Cmd {
	list := m.modelsFn
	return func() tea.Msg {
		models, err := list(context.Background(), id)
		return modelsMsg{id, models, err}
	}
}

// relist asks engine id for its models again.
func (m Model) relist(id string) (Model, tea.Cmd) {
	if m.modelsFn == nil {
		return m, nil
	}
	m.listed = maps.Clone(m.listed)
	m.listed[id] = listing{}
	return m, m.listCmd(id)
}

// gotModels takes an answer to a list asked for, and shows it in an
// open model picker.
func (m Model) gotModels(msg modelsMsg) Model {
	if l, asked := m.listed[msg.id]; !asked || l.done {
		return m
	}
	m.listed = maps.Clone(m.listed)
	m.listed[msg.id] = listing{msg.models, msg.err, true}
	if m.picker != nil && m.pickerFor == ctlModel && m.engine == msg.id {
		m = m.setModelItems()
	}
	return m
}

// followType puts the form on k's engine and model, until one is picked.
func (m Model) followType(k Kind) Model {
	if !m.chosen {
		on := m.runsOn[trackKinds[k]]
		m.engine, m.model = on.Engine, on.Model
	}
	return m
}

func (m Model) engineInfo(id string) (Engine, bool) {
	for _, e := range m.engines {
		if e.ID == id {
			return e, true
		}
	}
	return Engine{}, false
}

func engineName(id string) string {
	if e, ok := agents.ByID(id); ok {
		return e.Name
	}
	return id
}

func (m Model) engineText() string {
	if m.engine == "" {
		return "None added"
	}
	return engineName(m.engine)
}

func (m Model) modelText() string {
	if m.model != "" {
		return m.model
	}
	if e, ok := m.engineInfo(m.engine); ok && e.Default != "" {
		return "Default (" + e.Default + ")"
	}
	return "Default"
}

// engineProblem is why the track can't run on the engine shown, "" when
// it can.
func (m Model) engineProblem() string {
	if m.engine == "" {
		return string(tracks.ErrNoEngine)
	}
	if _, ok := m.engineInfo(m.engine); !ok {
		return engineName(m.engine) + " isn't added on the Engines tab. Add it there, or pick another agent."
	}
	return ""
}

// runsOnRow draws the agent and model fields side by side.
func (b *body) runsOnRow() {
	m := b.m
	engineW := min(24, max(8, b.width/3))
	gap := 2
	modelW := max(8, b.width-engineW-gap)
	b.hit(ctlEngine, 0, 0, engineW, 1)
	b.hit(ctlModel, 0, engineW+gap, modelW, 1)
	fg := theme.TextDefault
	if m.engine == "" {
		fg = theme.TextFaint
	}
	b.add(b.selectBox(ctlEngine, m.engineText(), fg, engineW) + "  " + b.selectBox(ctlModel, m.modelText(), theme.TextDefault, modelW))
	if problem := m.engineProblem(); problem != "" {
		b.wrapped(theme.StateWarningText, problem)
	}
}

// runsOnKey handles a key on the agent or model field, which share a
// row: ←/→ go between them, ↑/↓ leave the row.
func (m Model) runsOnKey(key string) (Model, tea.Cmd) {
	switch key {
	case "enter", "space":
		if m.focus == ctlEngine {
			return m.openEngines(), nil
		}
		return m.openModels()
	case "left":
		return m.setFocus(ctlEngine), nil
	case "right":
		return m.setFocus(ctlModel), nil
	case "up":
		return m.setFocus(ctlEngine).move(-1, false)
	case "down":
		return m.setFocus(ctlModel).move(1, false)
	}
	return m, nil
}

func (m Model) openEngines() Model {
	var items []widget.PickerItem
	marked := -1
	for i, e := range m.engines {
		items = append(items, widget.PickerItem{Label: e.Name})
		if e.ID == m.engine {
			marked = i
		}
	}
	if marked < 0 && m.engine != "" {
		items = append(items, widget.PickerItem{Label: engineName(m.engine), Detail: "not added"})
		marked = len(items) - 1
	}
	if len(items) == 0 {
		m.notice = string(tracks.ErrNoEngine)
		return m
	}
	p := widget.NewPicker("Agent", items, marked)
	m.picker, m.pickerFor = &p, ctlEngine
	return m
}

// enginePicked switches to the engine picked. Another engine starts on
// its default model, or the type's own when it's the type's engine.
func (m Model) enginePicked(i int) Model {
	if i >= len(m.engines) || m.engines[i].ID == m.engine {
		return m
	}
	m.engine, m.model, m.chosen = m.engines[i].ID, "", true
	if on := m.runsOn[trackKinds[m.kind]]; on.Engine == m.engine {
		m.model = on.Model
	}
	return m
}

func (m Model) openModels() (Model, tea.Cmd) {
	e, ok := m.engineInfo(m.engine)
	if !ok {
		m.notice = m.engineProblem()
		return m, nil
	}
	p := widget.NewPicker(e.Name+": model", nil, -1)
	p.Filter = true
	m.picker, m.pickerFor = &p, ctlModel
	if l := m.listed[e.ID]; e.Lists && l.done && l.err != nil {
		m, cmd := m.relist(e.ID)
		return m.setModelItems(), cmd
	}
	return m.setModelItems(), nil
}

// setModelItems lists Default, the engine's models and the ones the
// user added, marking the model shown.
func (m Model) setModelItems() Model {
	e, _ := m.engineInfo(m.engine)
	p := *m.picker
	m.picker = &p
	models := e.Models
	if e.Lists {
		switch l, asked := m.listed[e.ID]; {
		case !asked:
			p.Message, p.Problem = "Tracks can't list "+e.Name+"'s models here.", false
			return m
		case !l.done:
			p.Message, p.Problem = "Loading models…", false
			return m
		case l.err != nil:
			p.Message, p.Problem = "Couldn't list the models: "+l.err.Error()+" Enter tries again.", true
			return m
		}
		models = m.listed[e.ID].models
	}
	p.Message, p.Problem = "", false
	def := e.Name + " chooses"
	if e.Default != "" {
		def = e.Name + "'s default: " + e.Default
	}
	items := []widget.PickerItem{{Label: "Default", Detail: def}}
	for _, model := range models {
		items = append(items, widget.PickerItem{Label: model.ID, Detail: model.Label})
	}
	for _, model := range e.Added {
		if !slices.ContainsFunc(models, func(listed agents.Model) bool { return listed.ID == model }) {
			items = append(items, widget.PickerItem{Label: model, Detail: "added"})
		}
	}
	marked := 0
	for i, it := range items[1:] {
		if it.Label == m.model {
			marked = i + 1
		}
	}
	if marked == 0 && m.model != "" {
		items = append(items, widget.PickerItem{Label: m.model, Detail: notListed})
		marked = len(items) - 1
	}
	p.Marked, p.Cursor = marked, marked
	p.SetItems(items)
	return m
}

func (m Model) modelPicked(p *widget.Picker) Model {
	if p.Cursor >= len(p.Items) {
		return m
	}
	model := p.Items[p.Cursor].Label
	if p.Cursor == 0 {
		model = ""
	}
	if model != m.model {
		m.model, m.chosen = model, true
	}
	return m
}
