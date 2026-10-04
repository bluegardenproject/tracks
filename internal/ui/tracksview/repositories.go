package tracksview

import (
	"context"
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/ui/source"
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
	// formOffset is how many of the form's lines are scrolled away.
	formOffset int
	// hoverNew and hoverField are the New button and the form field
	// under the mouse; hoverField is -1 for none.
	hoverNew   bool
	hoverField formField
	form       repoForm
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
	m.repos.formOffset = 0
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
			return m.save(&l)
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
	case "ctrl+c":
		if in := f.input(f.focus); in != nil {
			m.repos.notice = notice{text: "Copied the value to the clipboard."}
			return m, tea.SetClipboard(in.Value())
		}
		return m, nil
	case "esc":
		return m.request(leave{kind: leaveForm})
	case "tab", "down":
		return m.leaveField(1)
	case "shift+tab", "up":
		return m.leaveField(-1)
	case "enter":
		return m.pressField(f.focus)
	case "space":
		if !f.isInput(f.focus) {
			return m.pressField(f.focus)
		}
	}
	in := f.input(f.focus)
	if in == nil {
		return m, nil
	}
	before := in.Value()
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	if in.Value() != before {
		delete(f.errs, f.errKey(f.focus))
	}
	return m, cmd
}

// repoPaste gives the focused input a paste, or the clipboard it read
// for Ctrl+V.
func (m Model) repoPaste(msg tea.Msg) (Model, tea.Cmd) {
	f := &m.repos.form
	in := f.input(f.focus)
	if !m.repos.editing || in == nil || m.repos.leaving != nil {
		return m, nil
	}
	var cmd tea.Cmd
	*in, cmd = in.Update(msg)
	delete(f.errs, f.errKey(f.focus))
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

// pressField acts on field: inputs move on to the next field, the
// checkbox toggles, a port mode moves on and buttons do what they say.
func (m Model) pressField(field formField) (Model, tea.Cmd) {
	f := &m.repos.form
	if f.isInput(field) {
		return m.leaveField(1)
	}
	if i, k, ok := field.server(); ok && i < len(f.servers) {
		switch k {
		case serverMode:
			f.servers[i].nextMode()
			delete(f.errs, f.errKey(field))
		case serverRemove:
			return m, f.removeServer(i)
		}
		return m, nil
	}
	switch field {
	case fieldAddSetup:
		return m, f.addSetup()
	case fieldRemoveSetup:
		return m, f.removeSetup()
	case fieldAddServer:
		return m, f.addServer()
	case fieldDrafts:
		f.drafts = !f.drafts
	case fieldSave, fieldPromptSave:
		if m.repos.leaving != nil {
			l := *m.repos.leaving
			return m.save(&l)
		}
		return m.save(nil)
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

// save saves the form once it's complete, leaving for then after.
func (m Model) save(then *leave) (Model, tea.Cmd) {
	if !m.repos.form.complete() {
		m.repos.leaving = nil
		return m, nil
	}
	return m, m.saveRepo(then)
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
			r.notice = notice{text: "Couldn't save: " + msg.err.Error(), err: true}
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
		m.repos.notice = notice{text: "Couldn't delete: " + msg.err.Error(), err: true}
		return m, nil
	}
	m.repos.notice = notice{text: fmt.Sprintf("Deleted %s.", msg.name)}
	m.repos.editing = false
	return m, m.loadRepos()
}
