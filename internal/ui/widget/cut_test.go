package widget

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestCut(t *testing.T) {
	for _, tc := range []struct {
		s     string
		width int
		want  string
	}{
		{"hello", 0, ""},
		{"hello", -3, ""},
		{"hello", 1, "…"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"日本語", 6, "日本語"},
		{"日本語", 5, "日本…"},
		{"日本語", 4, "日…"},
		{"日本語", 2, "…"},
		{"👩\u200d💻abc", 3, "👩\u200d💻…"},
		{"a-very-long-prefix-for-trains-123456", 22, "a-very-long-prefix-fo…"},
	} {
		got := Cut(tc.s, tc.width)
		if got != tc.want {
			t.Errorf("Cut(%q, %d) = %q, want %q", tc.s, tc.width, got, tc.want)
		}
		if tc.width >= 0 && lipgloss.Width(got) > tc.width {
			t.Errorf("Cut(%q, %d) = %q has width %d > %d", tc.s, tc.width, got, lipgloss.Width(got), tc.width)
		}
	}
}

func TestCutLeft(t *testing.T) {
	for _, tc := range []struct {
		s     string
		width int
		want  string
	}{
		{"hello", 0, ""},
		{"hello", 1, "…"},
		{"hello", 5, "hello"},
		{"hello", 4, "…llo"},
		{"日本語", 5, "…本語"},
		{"日本語", 4, "…語"},
		{"日本語", 2, "…"},
		{"abc👩\u200d💻", 3, "…👩\u200d💻"},
	} {
		got := CutLeft(tc.s, tc.width)
		if got != tc.want {
			t.Errorf("CutLeft(%q, %d) = %q, want %q", tc.s, tc.width, got, tc.want)
		}
		if tc.width >= 0 && lipgloss.Width(got) > tc.width {
			t.Errorf("CutLeft(%q, %d) = %q has width %d > %d", tc.s, tc.width, got, lipgloss.Width(got), tc.width)
		}
	}
}

func TestReadClipboard(t *testing.T) {
	orig := readClipboard
	t.Cleanup(func() { readClipboard = orig })

	readClipboard = func() (string, error) { return "copied text", nil }
	if msg, ok := ReadClipboard().(tea.ClipboardMsg); !ok || msg.Content != "copied text" {
		t.Fatalf("ReadClipboard() = %#v, want ClipboardMsg{Content: \"copied text\"}", msg)
	}

	readClipboard = func() (string, error) { return "", errors.New("unavailable") }
	if got := ReadClipboard(); got != tea.ReadClipboard() {
		t.Fatalf("ReadClipboard() fallback = %#v, want tea.ReadClipboard()", got)
	}

	in := NewInput(64, "")
	in.Focus()
	readClipboard = func() (string, error) { return "pasted-via-ctrl-v", nil }
	_, cmd := UpdateInput(in, tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("UpdateInput(Ctrl+V) returned nil cmd")
	}
	if msg, ok := cmd().(tea.ClipboardMsg); !ok || msg.Content != "pasted-via-ctrl-v" {
		t.Fatalf("UpdateInput(Ctrl+V) cmd = %#v, want ClipboardMsg", msg)
	}
}
