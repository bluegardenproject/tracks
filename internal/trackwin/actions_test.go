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
	if err := (tmux.Conf{DefaultTerminal: "screen-256color", Command: "true"}).Write(conf); err != nil {
		t.Fatal(err)
	}
	if err := c.NewSession(conf, "tracks", "Tracks", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	if keys, err := exec.Command("tmux", "-L", socket, "list-keys", "-T", "root", "MouseDown1Status").Output(); err != nil ||
		!strings.Contains(string(keys), "trackwin action '#{mouse_status_range}'") {
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

	const pointer = "1 message from @reviewer-1 on the relay: run `tracks relay read`."
	if err := trackwin.Paste(c, w.ID, pointer); err != nil {
		t.Fatal(err)
	}
	for range 50 {
		if screen, _ = c.CapturePane(w.Agent); strings.Count(strings.ReplaceAll(screen, "\n", ""), pointer) == 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The pane is narrow: the pointer wraps.
	if strings.Count(strings.ReplaceAll(screen, "\n", ""), pointer) != 2 {
		t.Errorf("the agent pane shows %q; want the pointer pasted, then cat's copy after Enter", screen)
	}
	if scrolled, err := c.InMode(w.Agent); err != nil || scrolled {
		t.Errorf("a pane not scrolled back reads as in a mode: %v, %v", scrolled, err)
	}
	if err := exec.Command("tmux", "-L", socket, "copy-mode", "-t", w.Agent).Run(); err != nil {
		t.Fatal(err)
	}
	if scrolled, err := c.InMode(w.Agent); err != nil || !scrolled {
		t.Errorf("a pane in copy mode reads as not: %v, %v", scrolled, err)
	}
	if err := c.CancelMode(w.Agent); err != nil {
		t.Fatal(err)
	}
	if scrolled, err := c.InMode(w.Agent); err != nil || scrolled {
		t.Errorf("a pane taken out of copy mode reads as in it: %v, %v", scrolled, err)
	}

	if err := trackwin.SetInfo(c, w.ID, "fix #12"); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.WindowOption(w.ID, trackwin.InfoOption); got != "fix ##12" {
		t.Errorf("SetInfo stored %q; want the # escaped, so tmux draws it and reads no style from it", got)
	}

	if err := trackwin.SetButtons(c, w.ID, "#[range=user|pr:0] PR ##3"); err != nil {
		t.Fatal(err)
	}
	if got, _ := c.WindowOption(w.ID, actions.ButtonsOption); got != "#[range=user|pr:0] PR ##3" {
		t.Errorf("SetButtons stored %q", got)
	}
}
