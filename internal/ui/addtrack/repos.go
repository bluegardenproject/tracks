package addtrack

import (
	"maps"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// selectRepos is the Repos field's selector while it's empty, and
// always its label.
const selectRepos = "Select repos"

// openRepos opens the picker on the repos: to tick any number of, or
// for Review to pick one. Typing filters them.
func (m Model) openRepos() Model {
	if len(m.repos) == 0 {
		return m
	}
	items := make([]widget.PickerItem, len(m.repos))
	for i, r := range m.repos {
		items[i] = widget.PickerItem{Label: r}
	}
	var p widget.Picker
	if m.kind == Review {
		p = widget.NewPicker("Repo", items, m.repo)
	} else {
		p = widget.NewPicker("Repos", items, -1)
		p.Ticked = make([]bool, len(m.repos))
		for i, r := range m.repos {
			p.Ticked[i] = m.picked[r]
		}
	}
	p.Filter = true
	m.picker, m.pickerFor = &p, m.repoControl()
	return m
}

// reposKey handles a key on the Repos field: left and right walk from
// the selector through the picked repos; Enter or Space opens the
// selector or removes the repo, as Backspace and Delete do.
func (m Model) reposKey(key string) (Model, tea.Cmd) {
	switch key {
	case "left":
		m.item = max(0, m.item-1)
	case "right":
		m.item = min(len(m.pickedRepos()), m.item+1)
	case "enter", "space":
		if m.item == 0 {
			return m.openRepos(), nil
		}
		m = m.unpick(m.item - 1)
	case "backspace", "delete":
		if m.item > 0 {
			m = m.unpick(m.item - 1)
		}
	case "up", "k":
		return m.move(-1, false)
	case "down", "j":
		return m.move(1, false)
	}
	return m, nil
}

// unpick removes the i-th picked repo; the cursor stays on a repo while
// any is left.
func (m Model) unpick(i int) Model {
	picked := m.pickedRepos()
	if i < 0 || i >= len(picked) {
		return m
	}
	m.picked = maps.Clone(m.picked)
	delete(m.picked, picked[i])
	if m.focus == ctlRepos {
		m.item = min(m.item, len(picked)-1)
	}
	return m
}

// reposHints are the Repos field's keys, for where its cursor is.
func (m Model) reposHints() []widget.KeyHelp {
	var keys []widget.KeyHelp
	if len(m.pickedRepos()) > 0 {
		keys = append(keys, widget.KeyHelp{Key: "←/→", Help: "the picked repos"})
	}
	if m.item == 0 {
		return append(keys, widget.KeyHelp{Key: "Enter", Help: "select repos"})
	}
	return append(keys, widget.KeyHelp{Key: "Enter", Help: "remove"})
}
