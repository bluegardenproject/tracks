package tracksview

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/ui/themecreator"
)

type fakeThemes struct {
	entries []theme.Entry
	chosen  []string
	saved   []theme.Theme
	created []string
}

func (f *fakeThemes) List() ([]theme.Entry, error) { return f.entries, nil }

func (f *fakeThemes) Choose(id string) (theme.Theme, error) {
	f.chosen = append(f.chosen, id)
	for _, e := range f.entries {
		if e.ID == id {
			return e.Theme, nil
		}
	}
	return theme.Theme{}, errors.New("no such theme")
}

func (f *fakeThemes) Save(t theme.Theme) error {
	f.saved = append(f.saved, t)
	return nil
}

func (f *fakeThemes) Create(name string, t theme.Theme) (theme.Theme, error) {
	f.created = append(f.created, name)
	t.ID, t.DisplayName, t.BuiltIn = theme.IDFrom(name), name, false
	f.entries = append(f.entries, theme.Entry{Theme: t, Path: t.ID + ".yaml"})
	return t, nil
}

func newFakeThemes() *fakeThemes {
	f := &fakeThemes{}
	for _, t := range theme.BuiltIns() {
		f.entries = append(f.entries, theme.Entry{Theme: t})
	}
	return f
}

// settingsTab opens the Settings tab on f's themes.
func openSettings(t *testing.T, f *fakeThemes) Model {
	t.Helper()
	m := New(Config{Version: "test", Theme: theme.Default(), Themes: f, ThemesDir: "/cfg/themes"})
	return settle(m, tea.WindowSizeMsg{Width: 120, Height: 40}, shiftTabKey)
}

var (
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
	downKey  = tea.KeyPressMsg{Code: tea.KeyDown}
)

func TestChoosingATheme(t *testing.T) {
	f := newFakeThemes()
	m := openSettings(t, f)
	view := plainView(m)
	for _, want := range []string{"General", "Fast Tracks", "Theme Creator", "Keys", "About", "default.yaml", "/cfg/themes"} {
		if !strings.Contains(view, want) {
			t.Errorf("Settings should show %q", want)
		}
	}
	m = settle(m, enterKey, enterKey)
	if m.settings.picker == nil || !strings.Contains(plainView(m), "default_light.yaml") {
		t.Fatal("Enter on the theme field should open the picker with every theme file")
	}
	m = settle(m, downKey, enterKey)
	if len(f.chosen) != 1 || f.chosen[0] != "default_light" || m.settings.picker != nil {
		t.Fatalf("chose %v, picker open %v; want default_light and the picker closed", f.chosen, m.settings.picker != nil)
	}
	if got := m.palette.Theme().ID; got != "default_light" {
		t.Errorf("window drawn in %s, want default_light", got)
	}
}

func TestInvalidThemesCantBeChosen(t *testing.T) {
	f := newFakeThemes()
	f.entries = append(f.entries, theme.Entry{Theme: theme.Theme{ID: "broken", DisplayName: "broken"}, Path: "/cfg/themes/broken.yaml", Err: errors.New("unknown token")})
	m := openSettings(t, f)
	m = settle(m, enterKey, enterKey)
	if !strings.Contains(plainView(m), "unknown token") {
		t.Error("the picker should show why a theme is invalid")
	}
	m = settle(m, downKey, downKey, enterKey)
	if len(f.chosen) != 0 || m.settings.picker == nil {
		t.Errorf("chose %v; want nothing chosen and the picker still open", f.chosen)
	}
	m = settle(m, tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft})
	if m.settings.picker != nil || len(f.chosen) != 0 {
		t.Error("a click outside the picker should close it without choosing")
	}
}

func TestCreatorLoadsThroughThePicker(t *testing.T) {
	m := openSettings(t, newFakeThemes())
	m = settle(m, downKey, downKey, enterKey, themecreator.LoadMsg{})
	if m.settings.picker == nil || m.settings.pickerFor != pickLoad {
		t.Fatal("the creator's Load should open the picker")
	}
	m = settle(m, downKey, enterKey)
	if got := m.settings.creator.Editing().ID; got != "default_light" || m.palette.Theme().ID != theme.DefaultID {
		t.Errorf("editing %s, using %s; want default_light loaded and the theme in use unchanged", got, m.palette.Theme().ID)
	}
}

func TestCreatorSavesThroughTheHost(t *testing.T) {
	f := newFakeThemes()
	m := openSettings(t, f)
	m = settle(m, themecreator.CreateMsg{Name: "Mine", Theme: theme.Default()})
	if len(f.created) != 1 || len(m.settings.themes) != 3 {
		t.Fatalf("created %v, %d themes listed; want Mine added", f.created, len(m.settings.themes))
	}
	if m.palette.Theme().ID != theme.DefaultID {
		t.Error("Save as new should not switch themes")
	}

	mine := m.settings.themes[2].Theme
	m = m.applyTheme(mine)
	m = settle(m, themecreator.SaveMsg{Theme: mine.With(theme.TextDefault, "#000001")})
	if len(f.saved) != 1 || m.palette.Theme().Value(theme.TextDefault) != "#000001" {
		t.Error("saving the theme in use should save it and redraw the window in it")
	}
}

func TestUnsavedEditsAskBeforeLeaving(t *testing.T) {
	m := openSettings(t, newFakeThemes())
	m = settle(m, downKey, downKey, enterKey)
	if !m.creatorFocused() {
		t.Fatal("Enter on Theme Creator should focus it")
	}
	m = settle(m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = typeText(m, "0")
	if !m.settings.creator.Dirty() {
		t.Fatal("editing a value should leave unsaved changes")
	}
	x, y := tabCell(t, m, "Station")
	m = settle(m, tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if m.tab != tabSettings || !strings.Contains(plainView(m), "Unsaved changes") {
		t.Fatal("clicking another tab with unsaved edits should ask first")
	}
	m = settle(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.tab != tabStation || m.settings.creator.Dirty() {
		t.Errorf("Discard: tab %d, dirty %v; want Station and the edits gone", m.tab, m.settings.creator.Dirty())
	}
}

// tabCell is a cell of the tab titled title.
func tabCell(t *testing.T, m Model, title string) (int, int) {
	t.Helper()
	for y, line := range strings.Split(plainView(m), "\n") {
		if x := strings.Index(line, title); x >= 0 {
			if i, ok := m.tabAt(len([]rune(line[:x])), y); ok && tabs[i].title == title {
				return len([]rune(line[:x])), y
			}
		}
	}
	t.Fatalf("tab %s not drawn", title)
	return 0, 0
}

func TestFastTracksEmptyState(t *testing.T) {
	m := openSettings(t, newFakeThemes())
	m = settle(m, downKey)
	view := plainView(m)
	for _, want := range []string{newButton.Label, "user defined template", fastTracksCreate} {
		if !strings.Contains(view, want) {
			t.Errorf("Fast Tracks should show %q", want)
		}
	}
	for y, line := range strings.Split(view, "\n") {
		if x := strings.Index(line, fastTracksCreate); x >= 0 {
			m = settle(m, tea.MouseClickMsg{X: len([]rune(line[:x])), Y: y, Button: tea.MouseLeft})
		}
	}
	if m.settings.notice.text != fastTracksLater || m.settings.editing {
		t.Errorf("clicking the dummy button: notice %q; want %q", m.settings.notice.text, fastTracksLater)
	}
}
