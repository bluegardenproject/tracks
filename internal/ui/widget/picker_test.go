package widget

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/ui/style"
)

func TestPickerFilter(t *testing.T) {
	p := NewPicker("Model", []PickerItem{{Label: "Default"}, {Label: "gpt-5", Detail: "Codex"}, {Label: "opus"}, {Label: "gpt-4", Detail: "Codex"}}, 0)
	p.Filter = true
	p.Size(80, 20)
	for _, k := range []string{"c", "o", "d", "x", "backspace"} {
		p.Key(k)
	}
	if p.Cursor != 1 {
		t.Fatalf("typing \"cod\" put the cursor on %d; want the first match, 1", p.Cursor)
	}
	p.Key("down")
	if p.Key("enter") != PickerChosen || p.Cursor != 3 {
		t.Errorf("down, enter chose %d; want the next match, 3", p.Cursor)
	}
	if p.Key("j") != PickerOpen || p.query != "codj" {
		t.Errorf("j should type into the filter, got %q", p.query)
	}
	if p.Key("enter") != PickerOpen {
		t.Error("Enter with nothing shown shouldn't choose")
	}

	p.Message, p.Problem = "the model list took too long", true
	if p.Key("enter") != PickerRetry || p.Key("esc") != PickerClosed {
		t.Error("an error message should retry on Enter and close on Esc")
	}
}

func TestPickerNarrowWidthAndTickBounds(t *testing.T) {
	p := NewPicker("Pick\x1b[31m", []PickerItem{
		{Label: "日本語のとても長いラベル名", Detail: "詳細\x1b]0;title\x07"},
		{Label: "short", Detail: "ok"},
	}, 0)
	w, h := p.Size(20, 10)
	out := p.View(style.New(theme.Default()), w, h)
	if strings.Contains(out, "\x1b]0;") {
		t.Errorf("OSC sequence leaked into Picker.View: %q", out)
	}
	for i, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got != w {
			t.Errorf("line %d width = %d, want %d: %q", i, got, w, line)
		}
	}

	// Ticked longer than Items must not panic.
	p.Ticked = make([]bool, 5)
	p.tick(3)

	// ANSI escapes in Message must not inflate Picker.Size width.
	plain := NewPicker("Pick", nil, 0)
	plain.Message = "timed out"
	pw, _ := plain.Size(80, 10)
	escaped := NewPicker("Pick", nil, 0)
	escaped.Message = "timed out\x1b]0;long-osc-title-that-should-not-widen-box\x07"
	ew, _ := escaped.Size(80, 10)
	if ew != pw {
		t.Errorf("escaped Message width = %d, want %d", ew, pw)
	}

	// Below minimum mark-fitting width returns empty; at minimum width fits exactly.
	if got := p.View(style.New(theme.Default()), 7, h); got != "" {
		t.Errorf("Picker.View width 7 = %q, want empty", got)
	}
	narrow := p.View(style.New(theme.Default()), 8, h)
	for i, line := range strings.Split(narrow, "\n") {
		if got := lipgloss.Width(line); got != 8 {
			t.Errorf("width 8 line %d width = %d, want 8: %q", i, got, line)
		}
	}
}
