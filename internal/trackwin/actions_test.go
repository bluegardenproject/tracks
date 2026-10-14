package trackwin_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

func TestPromptAndComments(t *testing.T) {
	socket := tmuxtest.Socket(t)
	c := tmux.New(socket)
	dir := t.TempDir()
	conf := filepath.Join(dir, "tmux.conf")
	if err := (tmux.Conf{DefaultTerminal: "screen-256color", Command: "true", Actions: actions.IDs}).Write(conf); err != nil {
		t.Fatal(err)
	}
	if err := c.NewSession(conf, "tracks", "Tracks", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	if keys, err := exec.Command("tmux", "-L", socket, "list-keys", "-T", "root", "MouseDown1Status").Output(); err != nil ||
		!strings.Contains(string(keys), "trackwin action "+actions.PRComments) {
		t.Errorf("tmux bound the footer click to %s (%v); want the action's branch", keys, err)
	}
	w, err := trackwin.Open(c, "tracks", trackwin.Spec{
		Name: "api", Dir: dir, Terminals: 1,
		Agent: trackwin.Process{Title: "agent", Command: "cat"},
	})
	if err != nil {
		t.Fatal(err)
	}

	const text = "-- work through the comments"
	if err := trackwin.Prompt(c, w.ID, text); err != nil {
		t.Fatal(err)
	}
	var screen string
	for range 50 {
		if screen, _ = c.CapturePane(w.Agent); strings.Count(screen, text) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Count(screen, text) != 2 {
		t.Errorf("the agent pane shows %q; want the line typed, then cat's copy after Enter", screen)
	}

	for _, tc := range []struct {
		c          actions.Comments
		open, sent string
	}{
		{actions.Comments{Open: 3, Sent: true}, "3", "1"},
		{actions.Comments{}, "", ""},
	} {
		if err := trackwin.SetComments(c, w.ID, tc.c); err != nil {
			t.Fatal(err)
		}
		open, _ := c.WindowOption(w.ID, actions.CommentsOption)
		sent, _ := c.WindowOption(w.ID, actions.SentOption)
		if open != tc.open || sent != tc.sent {
			t.Errorf("SetComments(%+v) left %q, %q", tc.c, open, sent)
		}
	}
}
