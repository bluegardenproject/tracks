package tracksview

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// typeFieldShown reports whether the Tracks section shows field: the
// effort only for an engine with effort levels.
func (m Model) typeFieldShown(field int) bool {
	return field != typeEffort || len(m.typeEngine().Efforts) > 0
}

// typeEffort is the selected type's own effort, "" for the engine's.
func (m Model) typeEffort() string {
	if d := m.settings.types.tracks.Get(string(m.selectedKind())); d != nil && d.Engine == m.typeEngine().ID && m.typeFieldShown(typeEffort) {
		return d.Effort
	}
	return ""
}

// effortText is the selected type's effort, as its field shows it.
func (m Model) effortText() string {
	if e := m.typeEffort(); e != "" {
		return e
	}
	return "Default"
}

// unknownEffort is beside an effort en doesn't list: one set by hand,
// or by a newer Tracks.
func unknownEffort(en agents.Engine) string { return "not a level " + en.Name + " lists" }

// effortListed reports whether the selected type's effort is Default or
// one of its engine's levels.
func (m Model) effortListed() bool {
	e := m.typeEffort()
	return e == "" || slices.Contains(m.typeEngine().Efforts, e)
}

// openEffortPicker lists the engine's effort levels after Default.
func (m Model) openEffortPicker() (Model, tea.Cmd) {
	en := m.typeEngine()
	items := []widget.PickerItem{{Label: "Default", Detail: en.Name + " chooses"}}
	for _, level := range en.Efforts {
		items = append(items, widget.PickerItem{Label: level})
	}
	marked := 0
	if cur := m.typeEffort(); cur != "" {
		if i := slices.Index(en.Efforts, cur); i >= 0 {
			marked = i + 1
		} else {
			items = append(items, widget.PickerItem{Label: cur, Detail: unknownEffort(en)})
			marked = len(items) - 1
		}
	}
	p := widget.NewPicker(kindTitle(m.selectedKind())+" tracks: effort", items, marked)
	p.Cursor = marked
	m.picker, m.pickerFor, m.pickerEngine = &p, pickTypeEffort, en.ID
	return m, nil
}

// effortPicked acts on what the effort picker did, for the engine it
// opened on: settings read meanwhile may have moved the type to another.
func (m Model) effortPicked(r widget.PickerResult) (Model, tea.Cmd) {
	switch r {
	case widget.PickerClosed:
		m.picker = nil
	case widget.PickerChosen:
		effort := ""
		if i := m.picker.Cursor; i > 0 {
			effort = m.picker.Items[i].Label
		}
		m.picker = nil
		model := ""
		if m.typeEngine().ID == m.pickerEngine {
			model = m.typeModel()
		}
		return m.setType(&settings.TrackType{Engine: m.pickerEngine, Model: model, Effort: effort})
	}
	return m, nil
}

// carriedEffort is the selected type's effort to keep beside a model
// picked for engine: none when the type no longer runs on it.
func (m Model) carriedEffort(engine string) string {
	if m.typeEngine().ID != engine {
		return ""
	}
	return m.typeEffort()
}
