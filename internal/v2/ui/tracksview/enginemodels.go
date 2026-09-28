package tracksview

import (
	"context"
	"maps"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// openModelPicker lists id's models to choose its default from; an
// engine that lists its own is asked the first time.
func (m Model) openModelPicker(id string) (Model, tea.Cmd) {
	en, ok := agents.ByID(id)
	if !ok || m.engines.settings.Get(id) == nil {
		return m, nil
	}
	p := widget.NewPicker(en.Name+": default model", nil, -1)
	p.Filter = true
	m.picker, m.pickerFor, m.pickerEngine = &p, pickModel, id
	if !en.ListsModels {
		return m.setModelItems(en.Models), nil
	}
	if models, ok := m.engines.models[id]; ok {
		return m.setModelItems(models), nil
	}
	return m.listModels()
}

// listModels asks the picker's engine for its models, looking for the
// CLI first if the last look didn't find it.
func (m Model) listModels() (Model, tea.Cmd) {
	id := m.pickerEngine
	en, _ := agents.ByID(id)
	m.picker.Message, m.picker.Problem = "Loading models…", false
	src, path := m.engineSource, m.engines.checks[id].found.Path
	if src == nil {
		return m, nil
	}
	return m, func() tea.Msg {
		if path == "" {
			found, err := src.Check(context.Background(), en)
			if err != nil {
				return engineModelsMsg{id: id, err: err}
			}
			path = found.Path
		}
		models, err := src.Models(context.Background(), path)
		return engineModelsMsg{id, models, err}
	}
}

func (m Model) engineModels(msg engineModelsMsg) Model {
	if msg.err == nil {
		m.engines.models = maps.Clone(m.engines.models)
		m.engines.models[msg.id] = msg.models
	}
	if m.picker == nil || m.pickerFor != pickModel || m.pickerEngine != msg.id {
		return m
	}
	if msg.err != nil {
		m.picker.Message, m.picker.Problem = "Couldn't list the models: "+msg.err.Error()+" Enter tries again.", true
		return m
	}
	m.picker.Message = ""
	return m.setModelItems(msg.models)
}

// setModelItems lists the engine's default, models and the user's
// added ones in the picker, marking the default in use.
func (m Model) setModelItems(models []agents.Model) Model {
	id := m.pickerEngine
	en, _ := agents.ByID(id)
	cur := m.engines.settings.Get(id)
	items := []widget.PickerItem{{Label: "Default", Detail: en.Name + " chooses"}}
	for _, model := range models {
		items = append(items, widget.PickerItem{Label: model.ID, Detail: model.Label})
	}
	marked := 0
	if cur != nil {
		for _, model := range cur.Models {
			items = append(items, widget.PickerItem{Label: model, Detail: "added"})
		}
		for i, it := range items[1:] {
			if cur.Model == it.Label {
				marked = i + 1
			}
		}
	}
	m.picker.Marked, m.picker.Cursor = marked, marked
	m.picker.SetItems(items)
	return m
}

// modelPicked acts on what the model picker did.
func (m Model) modelPicked(r widget.PickerResult) (Model, tea.Cmd) {
	switch r {
	case widget.PickerClosed:
		m.picker = nil
	case widget.PickerRetry:
		return m.listModels()
	case widget.PickerChosen:
		model := m.picker.Items[m.picker.Cursor].Label
		if m.picker.Cursor == 0 {
			model = ""
		}
		m.picker = nil
		return m.editEngine(m.pickerEngine, func(s *settings.Engine) { s.Model = model })
	}
	return m, nil
}
