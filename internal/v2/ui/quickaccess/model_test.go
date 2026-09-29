package quickaccess

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

func choose(msgs ...tea.Msg) string {
	var m tea.Model = New(theme.Default())
	m, _ = m.Update(tea.WindowSizeMsg{Width: Width, Height: Height()})
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m.(Model).Chosen()
}

func TestChoosing(t *testing.T) {
	tests := map[string]struct {
		msgs []tea.Msg
		want string
	}{
		"its key":           {[]tea.Msg{tea.KeyPressMsg{Code: 'n', Text: "n"}}, NewTrack},
		"Enter":             {[]tea.Msg{tea.KeyPressMsg{Code: tea.KeyEnter}}, NewTrack},
		"a click on it":     {[]tea.Msg{tea.MouseClickMsg{X: 5, Y: 1, Button: tea.MouseLeft}}, NewTrack},
		"Esc":               {[]tea.Msg{tea.KeyPressMsg{Code: tea.KeyEscape}}, ""},
		"a click beside":    {[]tea.Msg{tea.MouseClickMsg{X: 5, Y: 4, Button: tea.MouseLeft}}, ""},
		"f":                 {[]tea.Msg{tea.KeyPressMsg{Code: 'f', Text: "f"}}, TracksFilter},
		"↓ Enter":           {[]tea.Msg{tea.KeyPressMsg{Code: tea.KeyDown}, tea.KeyPressMsg{Code: tea.KeyEnter}}, TracksFilter},
		"a click on it too": {[]tea.Msg{tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft}}, TracksFilter},
	}
	for name, tt := range tests {
		if got := choose(tt.msgs...); got != tt.want {
			t.Errorf("%s chose %q, want %q", name, got, tt.want)
		}
	}
}
