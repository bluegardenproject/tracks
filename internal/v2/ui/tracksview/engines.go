package tracksview

import (
	"context"
	"maps"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// enginesTab is the Engines tab's state.
type enginesTab struct {
	settings settings.Engines
	loaded   bool
	loadErr  error
	checks   map[string]engineCheck    // by engine id
	models   map[string][]agents.Model // listed by a CLI, kept while the window runs
	mcp      map[string]mcpList
	editing  bool // focus is on the controls
	focus    engineControl
	hover    engineControl
	offset   int
	confirm  string          // the engine asking whether to be removed
	input    textinput.Model // a model id to add to Claude Code's list
	// saving is a save on its way; pending is another one due after it,
	// so saves land in order.
	saving, pending bool
	notice          notice
}

// engineCheck is what the last look for an engine's CLI found.
type engineCheck struct {
	checking bool
	found    agents.Found
	err      error
}

// mcpList is the last look at an engine's MCP servers.
type mcpList struct {
	checking, checked bool
	servers           []agents.MCPServer
	err               error
}

// engineControl is a control on the Engines tab; the zero one is none.
type engineControl struct {
	engine string
	kind   int
	index  int // the model, for ctlDropModel
}

// Engine controls.
const (
	ctlNone      = iota
	ctlAdd       // the empty state's framed button
	ctlCheck     // look for the CLI again
	ctlModel     // the default model field
	ctlDropModel // remove an added model
	ctlInput     // the model id to add
	ctlAddModel
	ctlAuto
	ctlRemove
	ctlConfirm // remove, confirmed
	ctlCancel
	ctlMCP     // check the MCP servers
	ctlDefault // make the engine the one new tracks run on
)

type (
	enginesMsg struct {
		settings settings.Engines
		err      error
		checks   map[string]engineCheck
	}
	// engineCheckedMsg is a look for id's CLI; add adds it when found.
	engineCheckedMsg struct {
		id    string
		check engineCheck
		add   bool
	}
	engineModelsMsg struct {
		id     string
		models []agents.Model
		err    error
	}
	engineMCPMsg struct {
		id      string
		servers []agents.MCPServer
		err     error
	}
	enginesSavedMsg struct{ err error }
)

const modelIDLimit = 120

func newEnginesTab() enginesTab {
	return enginesTab{checks: map[string]engineCheck{}, models: map[string][]agents.Model{}, mcp: map[string]mcpList{},
		input: widget.NewInput(modelIDLimit, "model id, such as claude-opus-5-5")}
}

// loadEngines reads the engines' settings and looks for every added
// CLI again, since one may have been installed or removed meanwhile.
func (m Model) loadEngines() (Model, tea.Cmd) {
	if m.engineSource == nil {
		return m, nil
	}
	e := &m.engines
	e.checks = maps.Clone(e.checks)
	for id, c := range e.checks {
		c.checking = true
		e.checks[id] = c
	}
	src := m.engineSource
	return m, func() tea.Msg {
		s, err := src.Load()
		checks := map[string]engineCheck{}
		for _, en := range agents.All {
			if s.Get(en.ID) != nil {
				found, err := src.Check(context.Background(), en)
				checks[en.ID] = engineCheck{found: found, err: err}
			}
		}
		return enginesMsg{s, err, checks}
	}
}

func (m Model) setEngines(msg enginesMsg) Model {
	e := &m.engines
	if !e.saving && !e.pending {
		e.settings = msg.settings
	}
	e.loaded, e.loadErr = true, msg.err
	checks := msg.checks
	for id, c := range e.checks {
		if _, ok := checks[id]; !ok && c.checking && e.settings.Get(id) == nil {
			checks[id] = c // an Add still looking
		}
	}
	e.checks = checks
	return m
}

// checkEngine looks for id's CLI, and adds the engine when add is set
// and it's there.
func (m Model) checkEngine(id string, add bool) (Model, tea.Cmd) {
	en, ok := agents.ByID(id)
	if !ok || m.engineSource == nil {
		return m, nil
	}
	e := &m.engines
	e.checks = maps.Clone(e.checks)
	e.checks[id] = engineCheck{checking: true}
	src := m.engineSource
	return m, func() tea.Msg {
		found, err := src.Check(context.Background(), en)
		return engineCheckedMsg{id, engineCheck{found: found, err: err}, add}
	}
}

func (m Model) engineChecked(msg engineCheckedMsg) (Model, tea.Cmd) {
	e := &m.engines
	e.checks = maps.Clone(e.checks)
	e.checks[msg.id] = msg.check
	if !msg.add || msg.check.err != nil || e.settings.Get(msg.id) != nil {
		return m, nil
	}
	en, _ := agents.ByID(msg.id)
	s := e.settings.Clone()
	s.Set(msg.id, &settings.Engine{})
	e.settings = s
	e.notice = notice{text: "Added " + en.Name + "."}
	if e.editing {
		m = m.focusEngine(engineControl{engine: msg.id, kind: ctlModel})
	}
	return m.saveEngines()
}

// editEngine changes id's settings through edit and saves them.
func (m Model) editEngine(id string, edit func(*settings.Engine)) (Model, tea.Cmd) {
	s := m.engines.settings.Clone()
	en := s.Get(id)
	if en == nil {
		return m, nil
	}
	edit(en)
	m.engines.settings = s
	return m.saveEngines()
}

// saveEngines writes the settings, after the save already running.
func (m Model) saveEngines() (Model, tea.Cmd) {
	e := &m.engines
	if m.engineSource == nil {
		return m, nil
	}
	if e.saving {
		e.pending = true
		return m, nil
	}
	e.saving = true
	src, s := m.engineSource, e.settings.Clone()
	return m, func() tea.Msg { return enginesSavedMsg{src.Save(s)} }
}

func (m Model) enginesSaved(msg enginesSavedMsg) (Model, tea.Cmd) {
	e := &m.engines
	e.saving = false
	if msg.err != nil {
		e.notice = notice{"Couldn't save the engines: " + msg.err.Error(), true}
	}
	if e.pending {
		e.pending = false
		return m.saveEngines()
	}
	return m, nil
}

// pressEngine does what control c is for.
func (m Model) pressEngine(c engineControl) (Model, tea.Cmd) {
	e := &m.engines
	en, _ := agents.ByID(c.engine)
	switch c.kind {
	case ctlAdd:
		return m.checkEngine(c.engine, true)
	case ctlCheck:
		return m.checkEngine(c.engine, e.settings.Get(c.engine) == nil)
	case ctlModel:
		return m.openModelPicker(c.engine)
	case ctlMCP:
		return m.checkMCP(c.engine)
	case ctlDropModel:
		return m.dropModel(c.engine, c.index)
	case ctlInput, ctlAddModel:
		return m.addModel(c.engine)
	case ctlAuto:
		cur := e.settings.Get(c.engine)
		if cur == nil {
			return m, nil
		}
		on := !cur.AutoMode()
		e.notice = notice{text: "Auto mode is off for " + en.Name + "."}
		if on {
			e.notice = notice{text: "Auto mode is on for " + en.Name + "."}
		}
		return m.editEngine(c.engine, func(s *settings.Engine) {
			s.Auto = nil
			if !on {
				s.Auto = &on
			}
		})
	case ctlDefault:
		s := e.settings.Clone()
		s.Default = c.engine
		e.settings = s
		e.notice = notice{text: "New tracks run on " + en.Name + "."}
		return m.saveEngines()
	case ctlRemove:
		e.confirm = c.engine
		return m.focusEngine(engineControl{engine: c.engine, kind: ctlCancel}), nil
	case ctlCancel:
		e.confirm = ""
		return m.focusEngine(engineControl{engine: c.engine, kind: ctlRemove}), nil
	case ctlConfirm:
		s := e.settings.Clone()
		s.Set(c.engine, nil)
		if s.Default == c.engine {
			s.Default = ""
		}
		e.settings, e.confirm = s, ""
		e.checks = maps.Clone(e.checks)
		delete(e.checks, c.engine)
		e.mcp = maps.Clone(e.mcp)
		delete(e.mcp, c.engine)
		e.notice = notice{text: "Removed " + en.Name + "."}
		m = m.focusEngine(engineControl{engine: c.engine, kind: ctlAdd})
		return m.saveEngines()
	}
	return m, nil
}

// addModel adds the typed model id to id's list.
func (m Model) addModel(id string) (Model, tea.Cmd) {
	e := &m.engines
	en, _ := agents.ByID(id)
	cur := e.settings.Get(id)
	model := strings.TrimSpace(e.input.Value())
	switch {
	case cur == nil:
		return m, nil
	case model == "":
		e.notice = notice{"Type a model id first.", true}
		return m, nil
	case strings.ContainsAny(model, " \t"):
		e.notice = notice{"A model id has no spaces.", true}
		return m, nil
	case slices.Contains(cur.Models, model) || slices.ContainsFunc(en.Models, func(b agents.Model) bool { return b.ID == model }):
		e.notice = notice{model + " is already listed.", true}
		return m, nil
	}
	e.input.SetValue("")
	e.notice = notice{text: "Added " + model + "."}
	return m.editEngine(id, func(s *settings.Engine) { s.Models = append(s.Models, model) })
}

// dropModel removes id's added model i; a default on it goes back to
// the engine's own.
func (m Model) dropModel(id string, i int) (Model, tea.Cmd) {
	cur := m.engines.settings.Get(id)
	if cur == nil || i < 0 || i >= len(cur.Models) {
		return m, nil
	}
	model := cur.Models[i]
	m.engines.notice = notice{text: "Removed " + model + "."}
	m, cmd := m.editEngine(id, func(s *settings.Engine) {
		s.Models = slices.Delete(s.Models, i, i+1)
		if s.Model == model {
			s.Model = ""
		}
	})
	if left := len(m.engines.settings.Get(id).Models); left > 0 {
		m = m.focusEngine(engineControl{engine: id, kind: ctlDropModel, index: min(i, left-1)})
	} else {
		m = m.focusEngine(engineControl{engine: id, kind: ctlInput})
	}
	return m, cmd
}

// focusEngine moves focus to c, into the input if c is it.
func (m Model) focusEngine(c engineControl) Model {
	e := &m.engines
	e.focus = c
	if c.kind == ctlInput {
		e.input.Focus()
	} else {
		e.input.Blur()
	}
	return m.scrollEngines()
}

// enginesListKey handles a key while no control has focus. ok is false
// for keys it doesn't use.
func (m Model) enginesListKey(key string) (_ Model, _ tea.Cmd, ok bool) {
	switch key {
	case "up", "k":
		return m.scrollEnginesBy(-1), nil, true
	case "down", "j":
		return m.scrollEnginesBy(1), nil, true
	case "enter", "right", "l":
		hits := m.engineHits()
		if len(hits) == 0 {
			return m, nil, true
		}
		m.engines.editing = true
		c := m.engines.focus
		if !slices.ContainsFunc(hits, func(h engineHit) bool { return h.c == c }) {
			c = hits[0].c
		}
		return m.focusEngine(c), nil, true
	}
	return m, nil, false
}

// enginesKey handles every key while a control has focus.
func (m Model) enginesKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	e := &m.engines
	key := msg.String()
	switch key {
	case "esc":
		if e.confirm != "" {
			return m.pressEngine(engineControl{engine: e.confirm, kind: ctlCancel})
		}
		e.editing = false
		e.input.Blur()
		return m, nil
	case "tab", "down":
		return m.moveEngineFocus(1), nil
	case "shift+tab", "up":
		return m.moveEngineFocus(-1), nil
	case "enter":
		return m.pressEngine(e.focus)
	}
	if e.focus.kind == ctlInput {
		var cmd tea.Cmd
		e.input, cmd = e.input.Update(msg)
		return m, cmd
	}
	if key == "space" {
		return m.pressEngine(e.focus)
	}
	return m, nil
}

// moveEngineFocus moves focus step controls on, stopping at the ends.
func (m Model) moveEngineFocus(step int) Model {
	hits := m.engineHits()
	if len(hits) == 0 {
		return m
	}
	at := slices.IndexFunc(hits, func(h engineHit) bool { return h.c == m.engines.focus })
	if at < 0 {
		return m.focusEngine(hits[0].c)
	}
	return m.focusEngine(hits[max(0, min(len(hits)-1, at+step))].c)
}

// enginesPaste pastes into the model id input when it has focus.
func (m Model) enginesPaste(msg tea.Msg) (Model, tea.Cmd) {
	e := &m.engines
	if !e.editing || e.focus.kind != ctlInput {
		return m, nil
	}
	var cmd tea.Cmd
	e.input, cmd = e.input.Update(msg)
	return m, cmd
}
