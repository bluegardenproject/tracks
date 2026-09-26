package tracksview

import (
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/themecreator"
	"github.com/bluegardenproject/tracks/internal/v2/ui/widget"
)

// Settings sections, in display order.
const (
	sectionGeneral    = iota
	sectionFastTracks // empty until Fast Tracks are built
	sectionCreator
	sectionKeys
	sectionAbout
)

var sectionTitles = []string{"General", "Fast Tracks", "Theme Creator", "Keys", "About"}

// settingsTab is the Settings tab's state.
type settingsTab struct {
	section, hover int
	editing        bool // focus is in the section
	themes         []theme.Entry
	themesErr      error
	fieldHover     bool // the mouse is on General's theme field
	fastHover      int  // the Fast Tracks button under the mouse
	keysOffset     int
	creator        themecreator.Model
	// picker, when set, is open over the window, choosing the theme
	// to use or, for the creator, to load.
	picker    *widget.Picker
	pickerFor int
	// leaving is where to go once the creator has let go.
	leaving *settingsLeave
	notice  notice
}

// settingsLeave is a section or tab to show after the creator asked
// about unsaved edits.
type settingsLeave struct {
	section int // -1 when leaving the tab
	tab     int
}

// What the picker chooses.
const (
	pickUse = iota
	pickLoad
)

type (
	themesMsg struct {
		entries []theme.Entry
		err     error
	}
	chosenMsg struct {
		theme theme.Theme
		err   error
	}
)

func newSettingsTab(applied theme.Theme) settingsTab {
	return settingsTab{hover: -1, creator: themecreator.New(applied, applied)}
}

func (m Model) loadThemes() tea.Cmd {
	if m.themeSource == nil {
		return nil
	}
	src := m.themeSource
	return func() tea.Msg {
		entries, err := src.List()
		return themesMsg{entries, err}
	}
}

func (m Model) setThemes(msg themesMsg) Model {
	s := &m.settings
	s.themes, s.themesErr = msg.entries, msg.err
	if s.picker != nil {
		s.picker.SetItems(m.pickerItems())
		s.picker.Marked = m.themeIndex(m.pickerMarks())
	}
	return m
}

// themeIndex is t's row in the list, -1 when it isn't listed.
func (m Model) themeIndex(t theme.Theme) int {
	for i, e := range m.settings.themes {
		if e.Err == nil && e.ID == t.ID && e.BuiltIn == t.BuiltIn {
			return i
		}
	}
	return -1
}

// fileName is the file e is, or would be for a built-in.
func fileName(e theme.Entry) string {
	if e.Path != "" {
		return filepath.Base(e.Path)
	}
	return e.ID + ".yaml"
}

func (m Model) pickerItems() []widget.PickerItem {
	items := make([]widget.PickerItem, len(m.settings.themes))
	for i, e := range m.settings.themes {
		items[i] = widget.PickerItem{Label: fileName(e), Detail: e.DisplayName}
		if e.BuiltIn {
			items[i].Detail += ", built-in"
		}
		if e.Err != nil {
			items[i].Problem = e.Err.Error()
		}
	}
	return items
}

// pickerMarks is the theme the picker marks: the one in use, or the
// one the creator edits.
func (m Model) pickerMarks() theme.Theme {
	if m.settings.pickerFor == pickLoad {
		return m.settings.creator.Editing()
	}
	return m.palette.Theme()
}

// openPicker shows the themes over the window, reading them again.
func (m Model) openPicker(purpose int) (Model, tea.Cmd) {
	s := &m.settings
	s.pickerFor = purpose
	title := "Choose a theme"
	if purpose == pickLoad {
		title = "Load a theme"
	}
	p := widget.NewPicker(title, m.pickerItems(), m.themeIndex(m.pickerMarks()))
	s.picker = &p
	return m, m.loadThemes()
}

// picked acts on what the picker did.
func (m Model) picked(r widget.PickerResult) (Model, tea.Cmd) {
	s := &m.settings
	switch r {
	case widget.PickerClosed:
		s.picker = nil
	case widget.PickerChosen:
		i := s.picker.Cursor
		s.picker = nil
		if s.pickerFor == pickLoad {
			return m, s.creator.Load(s.themes[i].Theme)
		}
		return m.choose(i)
	}
	return m, nil
}

func (m Model) pickerKey(key string) (Model, tea.Cmd) {
	return m.picked(m.settings.picker.Key(key))
}

// choose makes the theme at row i the one in use.
func (m Model) choose(i int) (Model, tea.Cmd) {
	s := &m.settings
	if i < 0 || i >= len(s.themes) || m.themeSource == nil {
		return m, nil
	}
	e := s.themes[i]
	if e.Err != nil {
		s.notice = notice{fmt.Sprintf("%s isn't a valid theme: %v", e.DisplayName, e.Err), true}
		return m, nil
	}
	src, id := m.themeSource, e.ID
	return m, func() tea.Msg {
		t, err := src.Choose(id)
		return chosenMsg{t, err}
	}
}

func (m Model) chosen(msg chosenMsg) Model {
	if msg.err != nil {
		m.settings.notice = notice{"Couldn't use the theme: " + msg.err.Error(), true}
		return m
	}
	m = m.applyTheme(msg.theme)
	m.settings.notice = notice{text: "Using " + msg.theme.DisplayName + "."}
	return m
}

// applyTheme draws the window in t.
func (m Model) applyTheme(t theme.Theme) Model {
	m.palette = style.New(t)
	m.settings.creator.SetApplied(t)
	return m
}

// showSection selects section i. The creator opens on the applied
// theme unless it holds edits.
func (m Model) showSection(i int) Model {
	s := &m.settings
	s.section, s.editing, s.leaving = i, false, nil
	s.creator.Blur()
	if i == sectionCreator && !s.creator.Dirty() {
		s.creator.Open(m.palette.Theme())
	}
	return m
}

// editSection moves focus into the section, if it takes focus.
func (m Model) editSection() (Model, tea.Cmd) {
	s := &m.settings
	switch s.section {
	case sectionFastTracks, sectionAbout:
		return m, nil
	case sectionCreator:
		s.editing = true
		return m, s.creator.Focus()
	}
	s.editing = true
	return m, nil
}

// settingsRequest goes to l, letting the creator ask first when it
// holds edits.
func (m Model) settingsRequest(l settingsLeave) (Model, tea.Cmd) {
	s := &m.settings
	if s.section == sectionCreator && s.editing && !s.creator.Leave() {
		s.leaving = &l
		return m, nil
	}
	return m.settingsFollow(l)
}

func (m Model) settingsFollow(l settingsLeave) (Model, tea.Cmd) {
	if l.section < 0 {
		m = m.showSection(m.settings.section)
		return m.switchTab(l.tab)
	}
	return m.showSection(l.section), nil
}

// creatorDone follows up on the creator letting go.
func (m Model) creatorDone() (Model, tea.Cmd) {
	if l := m.settings.leaving; l != nil {
		return m.settingsFollow(*l)
	}
	return m.showSection(m.settings.section), nil
}

// settingsListKey handles a key while the section list has focus. ok
// is false for keys it doesn't use.
func (m Model) settingsListKey(key string) (_ Model, _ tea.Cmd, ok bool) {
	s := m.settings
	switch key {
	case "up", "k":
		return m.showSection(max(0, s.section-1)), nil, true
	case "down", "j":
		return m.showSection(min(len(sectionTitles)-1, s.section+1)), nil, true
	case "enter", "right", "l":
		next, cmd := m.editSection()
		return next, cmd, true
	}
	return m, nil, false
}

// settingsKey handles every key while a section has focus.
func (m Model) settingsKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	s := &m.settings
	key := msg.String()
	if s.section == sectionCreator {
		var cmd tea.Cmd
		s.creator, cmd = s.creator.Update(msg)
		return m, cmd
	}
	if key == "esc" {
		return m.showSection(s.section), nil
	}
	switch s.section {
	case sectionGeneral:
		if key == "enter" || key == "space" {
			return m.openPicker(pickUse)
		}
	case sectionKeys:
		switch key {
		case "up", "k":
			s.keysOffset--
		case "down", "j":
			s.keysOffset++
		}
		s.keysOffset = max(0, min(s.keysOffset, len(m.keyLines())-m.sectionHeight()))
	}
	return m, nil
}

// creatorMsg handles what the creator asks of the host, and forwards
// everything else to it.
func (m Model) creatorMsg(msg tea.Msg) (Model, tea.Cmd) {
	s := &m.settings
	switch msg := msg.(type) {
	case themecreator.SaveMsg:
		if m.themeSource == nil {
			return m, nil
		}
		src, t := m.themeSource, msg.Theme
		return m, func() tea.Msg { return themecreator.SavedMsg{Theme: t, Err: src.Save(t)} }
	case themecreator.CreateMsg:
		if m.themeSource == nil {
			return m, nil
		}
		src := m.themeSource
		return m, func() tea.Msg {
			t, err := src.Create(msg.Name, msg.Theme)
			return themecreator.SavedMsg{Theme: t, Err: err}
		}
	case themecreator.SavedMsg:
		applied := m.palette.Theme()
		if msg.Err == nil && !msg.Theme.BuiltIn && !applied.BuiltIn && msg.Theme.ID == applied.ID {
			m = m.applyTheme(msg.Theme)
		}
		var cmd tea.Cmd
		s.creator, cmd = s.creator.Update(msg)
		return m, tea.Batch(cmd, m.loadThemes())
	case themecreator.DoneMsg:
		return m.creatorDone()
	case themecreator.StayMsg:
		s.leaving = nil
		return m, nil
	case themecreator.LoadMsg:
		return m.openPicker(pickLoad)
	}
	var cmd tea.Cmd
	s.creator, cmd = s.creator.Update(msg)
	return m, cmd
}
