package footer

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// TestActionsRow has tmux draw the row for a window with its options.
func TestActionsRow(t *testing.T) {
	socket := tmuxtest.Socket(t)
	c := tmux.New(socket)
	if err := c.NewSession("/dev/null", "tracks", "api", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	th := theme.Default()
	for name, value := range Options(th) {
		if out, err := exec.Command("tmux", "-L", socket, "set", "-g", name, value).CombinedOutput(); err != nil {
			t.Fatalf("set %s: %v %s", name, err, out)
		}
	}
	row := actionsRow(palette{th})
	draw := func(buttons string) string {
		t.Helper()
		if err := trackwin.SetButtons(c, "tracks:api", buttons); err != nil {
			t.Fatal(err)
		}
		if err := trackwin.SetInfo(c, "tracks:api", "api · active · Claude Code opus"); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", "tracks:api", row).Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}

	if got := strings.TrimSpace(styles.ReplaceAllString(draw(""), "")); got != "api · active · Claude Code opus" {
		t.Errorf("without buttons: %q, want the track's info", got)
	}
	got := draw(actions.Render([]actions.Button{
		{Index: 0, Repo: "a/b", Number: 3, Comments: actions.Comments{Open: 2}},
		{Index: 1, Repo: "a/b", Number: 7, Comments: actions.Comments{Open: 1}},
	}, false))
	if text := strings.TrimSpace(styles.ReplaceAllString(got, "")); text != "PR #3 · solve 2 comments   PR #7 · solve 1 comment" {
		t.Errorf("with buttons: %q", text)
	}
	if !strings.Contains(got, "#[bg="+th.Value(theme.ButtonVariant2Bg)+"]") {
		t.Errorf("the buttons' colour didn't come from the theme's options: %s", got)
	}
}
