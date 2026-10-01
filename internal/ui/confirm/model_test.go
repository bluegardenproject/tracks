package confirm

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/theme"
)

var escapes = regexp.MustCompile(`\x1b\[[0-9;:]*[a-zA-Z]`)

var question = Question{Title: "Close Tracks?", Text: []string{"Every window closes."}, Action: "Close Tracks"}

func run(msgs ...tea.Msg) (Model, bool) {
	var m tea.Model = New(theme.Default(), question)
	m, _ = m.Update(tea.WindowSizeMsg{Width: question.Width(), Height: question.Height()})
	quit := false
	for _, msg := range msgs {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		if cmd != nil {
			_, quit = cmd().(tea.QuitMsg)
		}
	}
	return m.(Model), quit
}

func TestAnswering(t *testing.T) {
	// The buttons are on the frame's fourth row, the action's from x 2
	// and Cancel's after it and the gap.
	actionX, cancelX := 3, 2+len(" Close Tracks ")+2+1
	tests := map[string]struct {
		msgs      []tea.Msg
		confirmed bool
		quits     bool
	}{
		"Enter":               {[]tea.Msg{tea.KeyPressMsg{Code: tea.KeyEnter}}, true, true},
		"y":                   {[]tea.Msg{tea.KeyPressMsg{Code: 'y', Text: "y"}}, true, true},
		"→ Enter":             {[]tea.Msg{tea.KeyPressMsg{Code: tea.KeyRight}, tea.KeyPressMsg{Code: tea.KeyEnter}}, false, true},
		"n":                   {[]tea.Msg{tea.KeyPressMsg{Code: 'n', Text: "n"}}, false, true},
		"Esc":                 {[]tea.Msg{tea.KeyPressMsg{Code: tea.KeyEscape}}, false, true},
		"a click on it":       {[]tea.Msg{tea.MouseClickMsg{X: actionX, Y: 3, Button: tea.MouseLeft}}, true, true},
		"a click on Cancel":   {[]tea.Msg{tea.MouseClickMsg{X: cancelX, Y: 3, Button: tea.MouseLeft}}, false, true},
		"a click beside":      {[]tea.Msg{tea.MouseClickMsg{X: actionX, Y: 1, Button: tea.MouseLeft}}, false, false},
		"another key":         {[]tea.Msg{tea.KeyPressMsg{Code: 'x', Text: "x"}}, false, false},
		"a right click on it": {[]tea.Msg{tea.MouseClickMsg{X: actionX, Y: 3, Button: tea.MouseRight}}, false, false},
	}
	for name, tt := range tests {
		m, quit := run(tt.msgs...)
		if m.Confirmed() != tt.confirmed || quit != tt.quits {
			t.Errorf("%s: confirmed %v, quit %v; want %v, %v", name, m.Confirmed(), quit, tt.confirmed, tt.quits)
		}
	}
}

func TestView(t *testing.T) {
	m, _ := run()
	view := escapes.ReplaceAllString(m.View().Content, "")
	lines := strings.Split(view, "\n")
	if len(lines) != question.Height() {
		t.Fatalf("view has %d lines, want %d:\n%s", len(lines), question.Height(), view)
	}
	for _, want := range []string{"Close Tracks?", "Every window closes.", " Close Tracks ", " Cancel ", "Esc cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if !strings.Contains(lines[3], "Cancel") {
		t.Errorf("the buttons should be on row 3:\n%s", view)
	}
}
