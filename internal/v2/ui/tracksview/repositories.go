package tracksview

import (
	"context"
	"errors"
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
)

// repoTab is the Repositories tab's state.
type repoTab struct {
	entries []source.RepositoryEntry
	err     error
	// selected is an index into entries, -1 for the New button.
	selected, offset, hover int
	// want is the repo to select once the list reloads.
	want    int64
	editing bool // focus is in the form
	form    repoForm
	// leaving holds what to do once unsaved changes are saved or
	// discarded, while asking.
	leaving *leave
	notice  notice
}

// leave is where the user wanted to go from a form with changes.
type leave struct {
	kind leaveKind
	row  int // for leaveRow
	tab  int // for leaveTab
}

type leaveKind int

const (
	leaveForm leaveKind = iota // back to the list
	leaveRow                   // to another repo
	leaveNew                   // to a new repo
	leaveTab                   // to another tab
)

type (
	reposMsg struct {
		entries []source.RepositoryEntry
		err     error
	}
	suggestMsg struct {
		path string
		repo source.RepositoryEntry
		err  error
	}
	repoSavedMsg struct {
		repo source.Repository
		err  error
		then *leave
	}
	repoDeletedMsg struct {
		name string
		err  error
	}
)

func (m Model) loadRepos() tea.Cmd {
	if m.repoSource == nil {
		return nil
	}
	src := m.repoSource
	return func() tea.Msg {
		entries, err := src.List(context.Background())
		return reposMsg{entries, err}
	}
}

func (m Model) setRepos(msg reposMsg) Model {
	r := &m.repos
	r.entries, r.err = msg.entries, msg.err
	if r.want != 0 {
		if i := slices.IndexFunc(r.entries, func(e source.RepositoryEntry) bool { return e.ID == r.want }); i >= 0 {
			r.selected = i
		}
		r.want = 0
	}
	r.selected = min(r.selected, len(r.entries)-1)
	if len(r.entries) == 0 {
		r.selected = -1
	}
	if !r.editing {
		m = m.showRepo(r.selected)
	}
	return m.scrollRepos()
}

// showRepo selects row i (-1 for New) and shows it in the form.
func (m Model) showRepo(i int) Model {
	m.repos.selected = i
	var e *source.RepositoryEntry
	if i >= 0 && i < len(m.repos.entries) {
		e = &m.repos.entries[i]
	}
	m.repos.form = newForm(e)
	m.repos.form.setWidth(m.inputWidth())
	return m.scrollRepos()
}

func (m Model) scrollRepos() Model {
	m.repos.offset = keepVisible(m.repos.selected, m.repos.offset, m.repoRows(), len(m.repos.entries))
	return m
}

// edit moves focus into the form, on field.
func (m Model) edit(field formField) (Model, tea.Cmd) {
	m.repos.editing = true
	return m, m.repos.form.setFocus(field)
}

// request goes to l, asking first when the form has changes.
func (m Model) request(l leave) (Model, tea.Cmd) {
	if m.repos.editing && m.repos.form.dirty() {
		m.repos.leaving = &l
		return m, nil
	}
	return m.follow(l)
}

// follow goes to l, dropping the form's state.
func (m Model) follow(l leave) (Model, tea.Cmd) {
	m.repos.leaving, m.repos.editing = nil, false
	switch l.kind {
	case leaveRow:
		return m.showRepo(l.row), nil
	case leaveNew:
		m = m.showRepo(-1)
		return m.edit(fieldName)
	case leaveTab:
		m = m.showRepo(m.repos.selected)
		return m.switchTab(l.tab)
	}
	return m.showRepo(m.repos.selected), nil
}

// repoListKey handles a key while the list has focus. ok is false for
// keys it doesn't use.
func (m Model) repoListKey(key string) (_ Model, _ tea.Cmd, ok bool) {
	r := m.repos
	switch key {
	case "up", "k":
		return m.showRepo(max(-1, r.selected-1)), nil, true
	case "down", "j":
		return m.showRepo(min(len(r.entries)-1, r.selected+1)), nil, true
	case "enter":
		next, cmd := m.edit(fieldName)
		return next, cmd, true
	case "n":
		next, cmd := m.follow(leave{kind: leaveNew})
		return next, cmd, true
	}
	return m, nil, false
}

// repoFormKey handles every key while the form has focus.
func (m Model) repoFormKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	f := &m.repos.form
	key := msg.String()
	switch {
	case m.repos.leaving != nil:
		l := *m.repos.leaving
		switch key {
		case "s", "enter":
			return m, m.saveRepo(&l)
		case "d":
			return m.follow(l)
		case "c", "esc":
			m.repos.leaving = nil
		}
		return m, nil
	case f.confirming:
		f.confirming = false
		if key == "y" || key == "enter" {
			return m, m.deleteRepo()
		}
		return m, nil
	}
	switch key {
	case "esc":
		return m.request(leave{kind: leaveForm})
	case "tab", "down":
		return m.leaveField(1)
	case "shift+tab", "up":
		return m.leaveField(-1)
	case "enter":
		return m.pressField(f.focus)
	case "space":
		if f.focus > fieldBase {
			return m.pressField(f.focus)
		}
	}
	if f.focus > fieldBase {
		return m, nil
	}
	before := f.inputs[f.focus].Value()
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	if f.inputs[f.focus].Value() != before {
		delete(f.errs, inputs[f.focus].key)
	}
	return m, cmd
}

// repoPaste types pasted text into the focused input.
func (m Model) repoPaste(msg tea.PasteMsg) (Model, tea.Cmd) {
	f := &m.repos.form
	if !m.repos.editing || f.focus > fieldBase || m.repos.leaving != nil {
		return m, nil
	}
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	delete(f.errs, inputs[f.focus].key)
	return m, cmd
}

// leaveField moves focus by delta, asking for a suggestion when it
// leaves a changed path.
func (m Model) leaveField(delta int) (Model, tea.Cmd) {
	f := &m.repos.form
	from := f.focus
	cmd := f.move(delta)
	if from == fieldPath {
		return m, tea.Batch(cmd, m.suggest())
	}
	return m, cmd
}

// pressField acts on field: inputs move on to the next field, the checkbox
// toggles and buttons do what they say.
func (m Model) pressField(field formField) (Model, tea.Cmd) {
	f := &m.repos.form
	switch field {
	case fieldPath, fieldName, fieldBase:
		return m.leaveField(1)
	case fieldDrafts:
		f.drafts = !f.drafts
	case fieldSave, fieldPromptSave:
		if m.repos.leaving != nil {
			l := *m.repos.leaving
			return m, m.saveRepo(&l)
		}
		return m, m.saveRepo(nil)
	case fieldDelete:
		f.confirming = true
	case fieldConfirmDelete:
		f.confirming = false
		return m, m.deleteRepo()
	case fieldCancelDelete:
		f.confirming = false
	case fieldPromptDiscard:
		if l := m.repos.leaving; l != nil {
			return m.follow(*l)
		}
	case fieldPromptCancel:
		m.repos.leaving = nil
	}
	return m, nil
}

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

func (m Model) saveRepo(then *leave) tea.Cmd {
	if m.repoSource == nil {
		return nil
	}
	src, r := m.repoSource, m.repos.form.repo()
	return func() tea.Msg {
		var saved source.Repository
		var err error
		if r.ID == 0 {
			saved, err = src.Add(context.Background(), r)
		} else {
			saved, err = src.Update(context.Background(), r)
		}
		return repoSavedMsg{saved, err, then}
	}
}

func (m Model) saved(msg repoSavedMsg) (Model, tea.Cmd) {
	r := &m.repos
	r.leaving = nil
	if msg.err != nil {
		if !r.form.fieldError(msg.err) {
			r.notice = notice{"Couldn't save: " + msg.err.Error(), true}
		}
		return m, nil
	}
	r.notice = notice{text: fmt.Sprintf("Saved %s.", msg.repo.Name)}
	r.want = msg.repo.ID
	then := leave{kind: leaveForm}
	if msg.then != nil {
		then = *msg.then
	}
	if then.kind == leaveForm || then.kind == leaveTab {
		r.editing = false
	}
	next, cmd := m.follow(then)
	return next, tea.Batch(cmd, next.loadRepos())
}

func (m Model) deleteRepo() tea.Cmd {
	f := m.repos.form
	if m.repoSource == nil || f.id == 0 {
		return nil
	}
	src, id, name := m.repoSource, f.id, f.original.Name
	return func() tea.Msg {
		return repoDeletedMsg{name, src.Delete(context.Background(), id)}
	}
}

func (m Model) deleted(msg repoDeletedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		m.repos.notice = notice{"Couldn't delete: " + msg.err.Error(), true}
		return m, nil
	}
	m.repos.notice = notice{text: fmt.Sprintf("Deleted %s.", msg.name)}
	m.repos.editing = false
	return m, m.loadRepos()
}
