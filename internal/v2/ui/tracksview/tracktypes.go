package tracksview

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// notListed is beside a track type's model the Engines tab no longer
// lists.
const notListed = "not listed on the Engines tab"

// typesState is Settings → Tracks: each track type's default agent and
// model. Its focus moves on to the Tracks History group below.
type typesState struct {
	tracks  settings.Tracks
	loadErr error
	kind    int // the selected type, in track.Kinds
	focus   int // typeRow to typeUnsaved
	hover   typeHit
	// saving is a save on its way; pending is another one due after it.
	saving, pending bool
}

// The Tracks section's controls: the types' row and fields, then
// Tracks History's.
const (
	typeRow = iota
	typeAgent
	typeModel
	typeAutoArchive
	typeUnsaved
)

// typeHit is a type in the row, or a field; -1 for none.
type typeHit struct{ kind, field int }

type (
	typesMsg struct {
		tracks  settings.Tracks
		engines settings.Engines
		err     error
	}
	typesSavedMsg struct{ err error }
)

// loadTypes reads the track types' defaults and the engines they
// follow, without asking the CLIs.
func (m Model) loadTypes() tea.Cmd {
	if m.typeSource == nil || m.engineSource == nil {
		return nil
	}
	types, engines := m.typeSource, m.engineSource
	return func() tea.Msg {
		t, err := types.Load()
		e, eerr := engines.Load()
		if err == nil {
			err = eerr
		}
		return typesMsg{t, e, err}
	}
}

func (m Model) setTypes(msg typesMsg) Model {
	t := &m.settings.types
	if !t.saving && !t.pending {
		t.tracks = msg.tracks
	}
	if e := &m.engines; !e.saving && !e.pending {
		e.settings = msg.engines
	}
	t.loadErr = msg.err
	return m
}

// setType makes d the selected type's defaults and saves them.
func (m Model) setType(d *settings.TrackType) (Model, tea.Cmd) {
	t := &m.settings.types
	kind := m.selectedKind()
	t.tracks.Set(string(kind), d)
	en, _ := agents.ByID(d.Engine)
	text := kindTitle(kind) + " tracks run on " + en.Name
	if d.Model != "" {
		text += ", model " + d.Model
	}
	m.settings.notice = notice{text: text + "."}
	return m.saveTypes()
}

func (m Model) saveTypes() (Model, tea.Cmd) {
	t := &m.settings.types
	if m.typeSource == nil {
		return m, nil
	}
	if t.saving {
		t.pending = true
		return m, nil
	}
	t.saving = true
	src, tracks := m.typeSource, t.tracks
	return m, func() tea.Msg { return typesSavedMsg{src.Save(tracks)} }
}

func (m Model) typesSaved(msg typesSavedMsg) (Model, tea.Cmd) {
	t := &m.settings.types
	t.saving = false
	if msg.err != nil {
		m.settings.notice = notice{text: "Couldn't save the track types: " + msg.err.Error(), err: true}
	}
	if t.pending {
		t.pending = false
		return m.saveTypes()
	}
	return m, nil
}

func (m Model) selectedKind() track.Kind { return track.Kinds[m.settings.types.kind] }

func kindTitle(k track.Kind) string { return strings.ToUpper(string(k[:1])) + string(k[1:]) }

// typeEngine is the engine the selected type runs on: its own, else
// what the Engines tab gives, else Claude Code.
func (m Model) typeEngine() agents.Engine {
	s := settings.Settings{Engines: m.engines.settings, Tracks: m.settings.types.tracks}
	id, _ := s.RunsOn(string(m.selectedKind()))
	if en, ok := agents.ByID(id); ok {
		return en
	}
	return agents.Claude
}

// typeModel is the selected type's own model, "" for the engine's
// default.
func (m Model) typeModel() string {
	if d := m.settings.types.tracks.Get(string(m.selectedKind())); d != nil {
		return d.Model
	}
	return ""
}

// defaultModel is what Default means for engine id's tracks.
func (m Model) defaultModel(id string) string {
	en, _ := agents.ByID(id)
	if conf := m.engines.settings.Get(id); conf != nil && conf.Model != "" {
		return en.Name + "'s default: " + conf.Model
	}
	return en.Name + " chooses"
}

// modelText is the selected type's model, as its field shows it.
func (m Model) modelText() string {
	if model := m.typeModel(); model != "" {
		return model
	}
	if conf := m.engines.settings.Get(m.typeEngine().ID); conf != nil && conf.Model != "" {
		return "Default (" + conf.Model + ")"
	}
	return "Default"
}

// listed reports whether the Engines tab lists model for en, as far as
// it knows: Cursor's list is only known once it's been read.
func (m Model) listed(en agents.Engine, model string) bool {
	if conf := m.engines.settings.Get(en.ID); conf != nil && slices.Contains(conf.Models, model) {
		return true
	}
	models, known := m.engines.models[en.ID]
	if !en.ListsModels {
		models, known = en.Models, true
	}
	return !known || slices.ContainsFunc(models, func(x agents.Model) bool { return x.ID == model })
}

// typesIntro is the section's text above the types.
func (m Model) typesIntro(width int) []string {
	about := "The agent and model each type of track starts with. A type you haven't set follows the Engines tab."
	lines := []string{m.fg(theme.TextDefault).Render("Track type default models")}
	lines = append(lines, strings.Split(m.fg(theme.TextMuted).Width(max(1, width)).Render(about), "\n")...)
	return append(lines, "")
}

func (m Model) typesView(width int) []string {
	history, _ := m.historyView(width)
	return append(append(m.typesBody(width), ""), history...)
}

// typesBody is the section above Tracks History.
func (m Model) typesBody(width int) []string {
	t := m.settings.types
	lit := func(field int) bool {
		return t.hover.field == field || m.settings.editing && t.focus == field
	}
	var row []string
	for i, k := range track.Kinds {
		row = append(row, m.listItem(kindTitle(k), t.hover.kind == i, i == t.kind))
	}
	en, fw := m.typeEngine(), m.themeFieldWidth()
	label := func(s string) string { return m.fg(theme.TextFaint).Render(pad(s, labelWidth)) }
	model := label("Model") + m.selectField(m.modelText(), fw, lit(typeModel))
	if name := m.typeModel(); name != "" && !m.listed(en, name) {
		model += m.fg(theme.StateWarningText).Render(cut("  "+notListed, max(0, width-labelWidth-fw-2)))
	}
	lines := append(m.typesIntro(width), strings.Join(row, " "), "",
		label("Agent")+m.selectField(en.Name, fw, lit(typeAgent)), model)
	if m.engines.settings.Get(en.ID) == nil {
		warn := en.Name + " isn't added on the Engines tab, so " + kindTitle(m.selectedKind()) + " tracks can't start."
		lines = append(lines, "")
		lines = append(lines, strings.Split(m.fg(theme.StateWarningText).Width(max(1, width)).Render(warn), "\n")...)
	}
	if t.loadErr != nil {
		lines = append(lines, "", m.fg(theme.StateDangerText).Render(cut("Couldn't read the settings: "+t.loadErr.Error(), width)))
	}
	return lines
}

// typeAt is what's at bx, by inside the section.
func (m Model) typeAt(bx, by int) typeHit {
	top := len(m.typesIntro(m.sectionWidth()))
	switch by {
	case top:
		x := 0
		for i, k := range track.Kinds {
			w := lipgloss.Width(kindTitle(k)) + 2
			if bx >= x && bx < x+w {
				return typeHit{i, -1}
			}
			x += w + 1
		}
	case top + 2, top + 3:
		if bx >= labelWidth && bx < labelWidth+m.themeFieldWidth()+2 {
			return typeHit{-1, typeAgent + by - top - 2}
		}
	}
	_, rows := m.historyView(m.sectionWidth())
	top = len(m.typesBody(m.sectionWidth())) + 1
	for field, row := range rows {
		w := m.themeFieldWidth() + 2
		if field == typeAutoArchive {
			w = len("[x] Auto-archive tracks")
		}
		if by == top+row && bx >= 0 && bx < w {
			return typeHit{-1, field}
		}
	}
	return typeHit{-1, -1}
}

func (m Model) typesClick(bx, by int) (Model, tea.Cmd) {
	t := &m.settings.types
	h := m.typeAt(bx, by)
	switch {
	case h.kind >= 0:
		t.kind, t.focus = h.kind, typeRow
	case h.field >= 0:
		t.focus = h.field
		return m.pressTypeField()
	}
	return m, nil
}

// pressTypeField acts on the focused field: a picker opens, the
// checkbox toggles.
func (m Model) pressTypeField() (Model, tea.Cmd) {
	switch m.settings.types.focus {
	case typeAutoArchive:
		return m.toggleAutoArchive()
	case typeUnsaved:
		return m.openUnsavedPicker()
	}
	return m.openTypePicker()
}

// typesKey handles a key while the Tracks section has focus.
func (m Model) typesKey(key string) (Model, tea.Cmd) {
	t := &m.settings.types
	switch key {
	case "left", "h":
		t.kind = max(0, t.kind-1)
	case "right", "l":
		t.kind = min(len(track.Kinds)-1, t.kind+1)
	case "up", "k", "shift+tab":
		t.focus = max(typeRow, t.focus-1)
	case "down", "j", "tab":
		t.focus = min(m.lastTypeField(), t.focus+1)
	case "enter", "space":
		if t.focus == typeRow {
			t.focus = typeAgent
			return m, nil
		}
		next, cmd := m.pressTypeField()
		next.settings.types.focus = min(next.settings.types.focus, next.lastTypeField())
		return next, cmd
	}
	return m, nil
}

// openTypePicker opens the picker for the focused field.
func (m Model) openTypePicker() (Model, tea.Cmd) {
	kind := kindTitle(m.selectedKind())
	if m.settings.types.focus == typeModel {
		return m.modelPicker(m.typeEngine(), pickTypeModel, kind+" tracks: model")
	}
	cur := m.typeEngine().ID
	items := make([]widget.PickerItem, len(agents.All))
	marked := 0
	for i, en := range agents.All {
		items[i] = widget.PickerItem{Label: en.Name}
		if m.engines.settings.Get(en.ID) == nil {
			items[i].Detail = "not added"
		}
		if en.ID == cur {
			marked = i
		}
	}
	p := widget.NewPicker(kind+" tracks: agent", items, marked)
	p.Cursor = marked
	m.picker, m.pickerFor = &p, pickAgent
	return m, nil
}

// agentPicked acts on what the agent picker did. Another agent starts
// on its default model.
func (m Model) agentPicked(r widget.PickerResult) (Model, tea.Cmd) {
	switch r {
	case widget.PickerClosed:
		m.picker = nil
	case widget.PickerChosen:
		en := agents.All[m.picker.Cursor]
		m.picker = nil
		model := ""
		if en.ID == m.typeEngine().ID {
			model = m.typeModel()
		}
		return m.setType(&settings.TrackType{Engine: en.ID, Model: model})
	}
	return m, nil
}
