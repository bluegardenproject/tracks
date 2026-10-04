package tracksview

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

// suggest asks what the path's checkout is, once per changed path.
func (m Model) suggest() tea.Cmd {
	f := m.repos.form
	path := f.value(fieldPath)
	if m.repoSource == nil || path == "" || path == f.suggested {
		return nil
	}
	src := m.repoSource
	return func() tea.Msg {
		r, err := src.Suggest(context.Background(), path)
		return suggestMsg{path, r, err}
	}
}

func (m Model) suggested(msg suggestMsg) Model {
	f := &m.repos.form
	if f.value(fieldPath) != msg.path {
		return m
	}
	f.suggested = msg.path
	if msg.err != nil {
		f.remote = ""
		var fe *source.FieldError
		if errors.As(msg.err, &fe) {
			f.errs[fe.Field] = fe.Message
		}
		return m
	}
	f.inputs[fieldPath].SetValue(msg.repo.Path)
	f.suggested = msg.repo.Path
	f.remote = msg.repo.Remote
	if f.value(fieldName) == "" {
		f.inputs[fieldName].SetValue(msg.repo.Name)
	}
	if f.value(fieldBase) == "" {
		f.inputs[fieldBase].SetValue(msg.repo.BaseBranch)
	}
	return m
}
