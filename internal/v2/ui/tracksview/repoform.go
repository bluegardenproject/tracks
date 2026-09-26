package tracksview

import (
	"errors"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// formField is something in the repo form that takes focus or clicks.
type formField int

const (
	fieldName formField = iota
	fieldPath
	fieldBase
	fieldDrafts
	fieldSave
	fieldDelete
	fieldConfirmDelete
	fieldCancelDelete
	fieldPromptSave
	fieldPromptDiscard
	fieldPromptCancel
)

// inputs are the form's text fields, in display order and indexed by
// field, with the key their errors come under.
var inputs = []struct {
	field            formField
	label, desc, key string
	limit            int
	placeholder      string
}{
	{fieldName, "Name", "How Tracks shows this repo. Spaces are fine.", "name", 64, "My repo"},
	{fieldPath, "Path", "The repo's main checkout on this computer.", "path", 4096, "~/src/my-repo"},
	{fieldBase, "Base branch", "New tracks branch off it; " + source.DefaultBase + " when left empty.", "base", 255, source.DefaultBase},
}

// repoForm edits one repo, or a new one when id is 0.
type repoForm struct {
	id       int64
	original source.Repository
	remote   string
	tracks   []string // active tracks in the repo
	inputs   []textinput.Model
	drafts   bool
	focus    formField
	errs     map[string]string
	// suggested is the path last suggested for, so leaving the path
	// field only asks again after it changed.
	suggested  string
	confirming bool // asking whether to delete
}

func newForm(e *source.RepositoryEntry) repoForm {
	f := repoForm{errs: map[string]string{}}
	for _, in := range inputs {
		f.inputs = append(f.inputs, widget.NewInput(in.limit, in.placeholder))
	}
	if e != nil {
		f.id, f.original, f.remote, f.tracks = e.ID, e.Repo, e.Remote, e.Tracks
		f.inputs[fieldName].SetValue(e.Name)
		f.inputs[fieldPath].SetValue(e.Path)
		f.inputs[fieldBase].SetValue(e.BaseBranch)
		f.drafts = e.DraftPRs
		f.suggested = e.Path
	}
	return f
}

func (f repoForm) value(field formField) string {
	return strings.TrimSpace(f.inputs[field].Value())
}

// repo is what the form would save.
func (f repoForm) repo() source.Repository {
	r := f.original
	r.ID, r.Path, r.Name, r.BaseBranch, r.DraftPRs = f.id, f.value(fieldPath), f.value(fieldName), f.value(fieldBase), f.drafts
	return r
}

func (f repoForm) dirty() bool {
	r := f.repo()
	return r.Path != f.original.Path || r.Name != f.original.Name || r.BaseBranch != f.original.BaseBranch ||
		r.DraftPRs != f.original.DraftPRs
}

// order is what Tab moves through.
func (f repoForm) order() []formField {
	order := []formField{fieldName, fieldPath, fieldBase, fieldDrafts}
	if f.dirty() {
		order = append(order, fieldSave)
	}
	if f.id != 0 {
		order = append(order, fieldDelete)
	}
	return order
}

// setFocus moves focus to field, focusing its input if it has one.
func (f *repoForm) setFocus(field formField) tea.Cmd {
	f.focus = field
	var cmd tea.Cmd
	for i := range f.inputs {
		if formField(i) == field {
			cmd = f.inputs[i].Focus()
		} else {
			f.inputs[i].Blur()
		}
	}
	return cmd
}

// move shifts focus by delta through the order, wrapping.
func (f *repoForm) move(delta int) tea.Cmd {
	order := f.order()
	i := slices.Index(order, f.focus)
	if i < 0 {
		i = 0
		delta = max(delta, 0)
	}
	return f.setFocus(order[(i+delta+len(order))%len(order)])
}

func (f *repoForm) setWidth(width int) {
	for i := range f.inputs {
		f.inputs[i].SetWidth(max(1, width-1))
	}
}

// fieldError shows err next to its field, or returns false for errors
// that aren't about one field.
func (f *repoForm) fieldError(err error) bool {
	var fe *source.FieldError
	if !errors.As(err, &fe) {
		return false
	}
	f.errs[fe.Field] = fe.Message
	for _, in := range inputs {
		if in.key == fe.Field {
			f.setFocus(in.field)
		}
	}
	return true
}
