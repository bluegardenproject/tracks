package tracksview

import (
	"slices"

	tea "charm.land/bubbletea/v2"
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
	if d := m.settings.types.tracks.Get(string(m.selectedKind())); d != nil && d.Engine == m.typeEngine().ID {
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

// openEffortPicker lists the engine's effort levels after Default.
func (m Model) openEffortPicker() (Model, tea.Cmd) {
	en := m.typeEngine()
	items := []widget.PickerItem{{Label: "Default", Detail: en.Name + " chooses"}}
	for _, level := range en.Efforts {
		items = append(items, widget.PickerItem{Label: level})
	}
	marked := 0
	if i := slices.Index(en.Efforts, m.typeEffort()); i >= 0 {
		marked = i + 1
	}
	p := widget.NewPicker(kindTitle(m.selectedKind())+" tracks: effort", items, marked)
	p.Cursor = marked
	m.picker, m.pickerFor = &p, pickTypeEffort
	return m, nil
}

// effortPicked acts on what the effort picker did.
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
		return m.setType(&settings.TrackType{Engine: m.typeEngine().ID, Model: m.typeModel(), Effort: effort})
	}
	return m, nil
}
