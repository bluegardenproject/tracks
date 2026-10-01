package addtrack

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

func (m Model) openCandor() Model {
	items := make([]widget.PickerItem, 0, track.MaxCandor)
	for level := track.MinCandor; level <= track.MaxCandor; level++ {
		label := track.CandorLabel(level)
		if level == track.DefaultCandor {
			label += " (default)"
		}
		items = append(items, widget.PickerItem{Label: strconv.Itoa(level), Detail: label})
	}
	p := widget.NewPicker("Candor", items, m.candor-track.MinCandor)
	m.picker, m.pickerFor = &p, ctlCandor
	return m
}

func (m Model) pickerKey(key string) (Model, tea.Cmd) {
	p := *m.picker
	m.picker = &p
	return m.pickerDone(p.Key(key))
}

// pickerDone follows up on the picker closing. The repos ticked stay
// ticked however it closed.
func (m Model) pickerDone(r widget.PickerResult) (Model, tea.Cmd) {
	if r == widget.PickerRetry && m.pickerFor == ctlModel {
		m, cmd := m.relist(m.engine)
		return m.setModelItems(), cmd
	}
	if r != widget.PickerChosen && r != widget.PickerClosed {
		return m, nil
	}
	p := m.picker
	m.picker = nil
	switch {
	case m.pickerFor == ctlRepos:
		m.picked = map[string]bool{}
		for i, on := range p.Ticked {
			if on {
				m.picked[m.repos[i]] = true
			}
		}
		delete(m.errs, ctlRepos)
	case r == widget.PickerClosed:
	case m.pickerFor == ctlRepo:
		m.repo = p.Cursor
		delete(m.errs, ctlRepo)
	case m.pickerFor == ctlCandor:
		m.candor = p.Cursor + track.MinCandor
	case m.pickerFor == ctlEngine:
		m = m.enginePicked(p.Cursor)
	case m.pickerFor == ctlModel:
		m = m.modelPicked(p)
	}
	return m, nil
}
