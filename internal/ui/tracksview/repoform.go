package tracksview

import (
	"errors"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/ui/source"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// formField is something in the repo form that takes focus or clicks.
// A dev server's fields come after the named ones: see serverField.
type formField int

const (
	fieldName formField = iota
	fieldPath
	fieldBase
	fieldDrafts
	fieldSetup
	fieldRemoveSetup
	fieldAddServer
	fieldAddSetup
	fieldSave
	fieldDelete
	fieldConfirmDelete
	fieldCancelDelete
	fieldPromptSave
	fieldPromptDiscard
	fieldPromptCancel
	fieldServers
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
	setupOn  bool // the Setup section is shown
	setup    textinput.Model
	servers  []serverForm
	width    int // of the inputs, for ones added later
	focus    formField
	errs     map[string]string
	// suggested is the path last suggested for, so leaving the path
	// field only asks again after it changed.
	suggested  string
	confirming bool // asking whether to delete
}

func newForm(e *source.RepositoryEntry) repoForm {
	f := repoForm{errs: map[string]string{}, setup: widget.NewInput(4096, "pnpm install")}
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
		f.setupOn = e.Setup != ""
		f.setup.SetValue(e.Setup)
		for _, d := range e.Servers {
			f.servers = append(f.servers, newServerForm(d))
		}
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
	r.Setup = ""
	if f.setupOn {
		r.Setup = strings.TrimSpace(f.setup.Value())
	}
	r.Servers = nil
	for _, s := range f.servers {
		r.Servers = append(r.Servers, s.server())
	}
	return r
}

func (f repoForm) dirty() bool {
	r := f.repo()
	return r.Path != f.original.Path || r.Name != f.original.Name || r.BaseBranch != f.original.BaseBranch ||
		r.DraftPRs != f.original.DraftPRs || r.Setup != f.original.Setup || !slices.Equal(r.Servers, f.original.Servers)
}

// order is what Tab moves through, in display order.
func (f repoForm) order() []formField {
	order := []formField{fieldName, fieldPath, fieldBase, fieldDrafts}
	if f.setupOn {
		order = append(order, fieldSetup, fieldRemoveSetup)
	}
	for i, s := range f.servers {
		for _, k := range s.fields() {
			order = append(order, serverField(i, k))
		}
	}
	if len(f.servers) > 0 {
		order = append(order, fieldAddServer)
	}
	if !f.setupOn {
		order = append(order, fieldAddSetup)
	}
	if len(f.servers) == 0 {
		order = append(order, fieldAddServer)
	}
	if f.dirty() {
		order = append(order, fieldSave)
	}
	if f.id != 0 {
		order = append(order, fieldDelete)
	}
	return order
}

// input is field's text input, nil for fields that aren't one.
func (f *repoForm) input(field formField) *textinput.Model {
	switch {
	case field <= fieldBase:
		return &f.inputs[field]
	case field == fieldSetup:
		return &f.setup
	}
	if i, k, ok := field.server(); ok && i < len(f.servers) {
		return f.servers[i].input(k)
	}
	return nil
}

// isInput reports whether field is a text input.
func (f repoForm) isInput(field formField) bool { return f.input(field) != nil }

// errKey is the key field's errors come under.
func (f repoForm) errKey(field formField) string {
	switch {
	case field <= fieldBase:
		return inputs[field].key
	case field == fieldSetup:
		return "setup"
	}
	if i, k, ok := field.server(); ok {
		return serverErrKey(i, k)
	}
	return ""
}

// fieldFor is the field whose errors come under key.
func (f repoForm) fieldFor(key string) (formField, bool) {
	order := f.order()
	for _, field := range order {
		if f.isInput(field) && f.errKey(field) == key {
			return field, true
		}
	}
	for _, field := range order {
		if _, k, ok := field.server(); ok && k == serverMode && f.errKey(field) == key {
			return field, true
		}
	}
	return 0, false
}

// eachInput calls fn on every text input.
func (f *repoForm) eachInput(fn func(*textinput.Model)) {
	for i := range f.inputs {
		fn(&f.inputs[i])
	}
	fn(&f.setup)
	for i := range f.servers {
		for k := range f.servers[i].inputs {
			fn(&f.servers[i].inputs[k])
		}
	}
}

// setFocus moves focus to field, focusing its input if it has one.
func (f *repoForm) setFocus(field formField) tea.Cmd {
	f.focus = field
	f.eachInput(func(in *textinput.Model) { in.Blur() })
	if in := f.input(field); in != nil {
		return in.Focus()
	}
	return nil
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
	f.width = max(1, width-1)
	f.eachInput(func(in *textinput.Model) { in.SetWidth(f.width) })
}

// fieldError shows err next to its field, or returns false for errors
// that aren't about one field.
func (f *repoForm) fieldError(err error) bool {
	var fe *source.FieldError
	if !errors.As(err, &fe) {
		return false
	}
	f.errs[fe.Field] = fe.Message
	if field, ok := f.fieldFor(fe.Field); ok {
		f.setFocus(field)
	}
	return true
}

// complete reports whether the form can be saved: a shown Setup needs
// its command. Otherwise it says so under the field.
func (f *repoForm) complete() bool {
	if f.setupOn && strings.TrimSpace(f.setup.Value()) == "" {
		f.errs["setup"] = "Enter the setup command, or remove the setup."
		f.setFocus(fieldSetup)
		return false
	}
	return true
}

// addSetup shows the Setup section, focused.
func (f *repoForm) addSetup() tea.Cmd {
	f.setupOn = true
	return f.setFocus(fieldSetup)
}

// removeSetup hides the Setup section and drops its command.
func (f *repoForm) removeSetup() tea.Cmd {
	f.setupOn = false
	f.setup.SetValue("")
	delete(f.errs, "setup")
	return f.setFocus(fieldAddSetup)
}

// addServer adds an empty dev server below the others, focused.
func (f *repoForm) addServer() tea.Cmd {
	s := newServerForm(source.DevServer{})
	for k := range s.inputs {
		s.inputs[k].SetWidth(f.width)
	}
	f.servers = append(f.servers, s)
	return f.setFocus(serverField(len(f.servers)-1, serverName))
}

// removeServer drops dev server i. Errors of the servers are dropped
// too, since their indexes shift.
func (f *repoForm) removeServer(i int) tea.Cmd {
	f.servers = slices.Delete(f.servers, i, i+1)
	for key := range f.errs {
		if strings.HasPrefix(key, "servers.") {
			delete(f.errs, key)
		}
	}
	if i < len(f.servers) {
		return f.setFocus(serverField(i, serverName))
	}
	return f.setFocus(fieldAddServer)
}
