package footer

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
)

// TestActionsRow has tmux draw the row for a window with its options.
func TestActionsRow(t *testing.T) {
	socket := tmuxtest.Socket(t)
	c := tmux.New(socket)
	if err := c.NewSession("/dev/null", "tracks", "api", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	row := actionsRow(palette{theme.Default()})
	draw := func(open, sent string) string {
		t.Helper()
		for name, value := range map[string]string{actions.CommentsOption: open, actions.SentOption: sent} {
			if err := c.SetWindowOption("tracks:api", name, value); err != nil {
				t.Fatal(err)
			}
		}
		out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", "tracks:api", row).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(styles.ReplaceAllString(string(out), ""))
	}
	for _, tc := range []struct{ open, sent, want string }{
		{"", "", ""},
		{"1", "", "1 open comment · solve now"},
		{"3", "", "3 open comments · solve now"},
		{"3", "1", "3 open comments · sent to the agent"},
	} {
		if got := draw(tc.open, tc.sent); got != tc.want {
			t.Errorf("with %q open, sent %q: %q, want %q", tc.open, tc.sent, got, tc.want)
		}
	}
	if !strings.Contains(row, "#[range=user|"+actions.PRComments+"]") {
		t.Error("the button isn't a range named after its action")
	}
}
