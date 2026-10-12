package tracksview

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

type titled struct{ id, title string }

// withTitles is a Station whose renames land in got.
func withTitles(got *[]titled) Model {
	tracks := demoTracks(2)
	tracks[0].ID, tracks[1].ID = "a", "b"
	tracks[1].Title = "Rate bug"
	m := New(Config{Version: "test", Theme: theme.Default(), SetTitle: func(id, title string) error {
		*got = append(*got, titled{id, title})
		return nil
	}})
	return update(m, tea.WindowSizeMsg{Width: 120, Height: 40}, tracksMsg{tracks: tracks})
}

func TestRenameSetsTheTitle(t *testing.T) {
	var got []titled
	m := withTitles(&got)
	m = clickButton(t, m, "Rename")
	if m.station.rename == nil || m.station.rename.input.Value() != "track-01" {
		t.Fatalf("Rename should open on the shown name, got %+v", m.station.rename)
	}
	if row := hintRow(m); !strings.Contains(row, "Enter rename") {
		t.Errorf("hint row = %q, want the box's keys", row)
	}
	m = typeText(m, "-x")
	m = run(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.station.rename != nil || len(got) != 1 || got[0] != (titled{"a", "track-01-x"}) {
		t.Errorf("renamed %+v, box %v; want a to track-01-x, the box closed", got, m.station.rename)
	}
}

func TestRenameClearButtonEmptiesTheInput(t *testing.T) {
	var got []titled
	m := withTitles(&got)
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = run(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if v := m.station.rename.input.Value(); v != "Rate bug" {
		t.Fatalf("input = %q, want the title", v)
	}
	x, y := cellOf(t, m, "✕ ]")
	m = run(m, tea.MouseClickMsg{X: x - 1, Y: y, Button: tea.MouseLeft})
	if r := m.station.rename; r.input.Value() != "" || r.input.Position() != 0 {
		t.Fatalf("after ✕: %q at %d, want empty at 0", r.input.Value(), r.input.Position())
	}
	m = run(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(got) != 1 || got[0] != (titled{"b", ""}) {
		t.Errorf("renamed %+v, want b back to its slug", got)
	}
}

func TestRenameClickPlacesTheCursor(t *testing.T) {
	var got []titled
	m := withTitles(&got)
	m = run(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	x, y := cellOf(t, m, "[track-01")
	m = run(m, tea.MouseClickMsg{X: x + 3, Y: y, Button: tea.MouseLeft})
	if p := m.station.rename.input.Position(); p != 3 {
		t.Fatalf("clicked the c, cursor at %d, want 3", p)
	}
	m = typeText(m, "Z")
	m = run(m, tea.MouseClickMsg{X: x + 40, Y: y, Button: tea.MouseLeft})
	m = typeText(m, "!")
	if v := m.station.rename.input.Value(); v != "traZck-01!" {
		t.Errorf("input = %q, want traZck-01!: typed at the click, then at the end", v)
	}
}

func TestRenameCancels(t *testing.T) {
	var got []titled
	m := withTitles(&got)
	for _, close := range []tea.Msg{
		tea.KeyPressMsg{Code: tea.KeyEscape},
		tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft},
		tea.KeyPressMsg{Code: tea.KeyEnter},
	} {
		m = run(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
		m = run(m, close)
		if m.station.rename != nil {
			t.Errorf("%T left the box open", close)
		}
	}
	m = run(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = clickButton(t, m, "Cancel")
	if m.station.rename != nil || len(got) != 0 {
		t.Errorf("cancelled and unchanged, yet renamed %+v", got)
	}
}

func TestRenameScrollsToTheCursor(t *testing.T) {
	in := widget.NewInput(titleLimit, "")
	in.SetValue(strings.Repeat("abcdef", 5))
	in.CursorEnd()
	r := renameBox{input: in}
	r.scroll(10)
	if r.offset != 21 || r.runeAt(0) != 21 || r.runeAt(20) != 30 {
		t.Errorf("offset %d, first shown %d, past the end %d; want 21, 21, 30", r.offset, r.runeAt(0), r.runeAt(20))
	}
	r.input.CursorStart()
	r.scroll(10)
	if r.offset != 0 {
		t.Errorf("cursor at the start, offset %d", r.offset)
	}
	r.input.SetValue("abc")
	r.input.CursorEnd()
	r.offset = 2
	r.scroll(10)
	if r.offset != 0 {
		t.Errorf("short text from offset %d, want all of it shown", r.offset)
	}
}

func TestRenameNeedsTheDaemon(t *testing.T) {
	m := update(New(Config{Version: "test", Theme: theme.Default()}), tea.WindowSizeMsg{Width: 120, Height: 40},
		tracksMsg{tracks: []source.Track{{ID: "a", Number: 1, Name: "a", Status: track.Active}}})
	m = run(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.station.rename != nil {
		t.Error("Rename opened without a way to save")
	}
}
