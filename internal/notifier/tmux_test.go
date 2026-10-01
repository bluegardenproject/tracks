package notifier

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// TestRealTmux rings a track's bell on a real tmux server, with a
// control-mode client showing the Tracks window.
func TestRealTmux(t *testing.T) {
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
	w, err := trackwin.Open(c, "tracks", trackwin.Spec{Track: "a", Name: "rate-bug", Dir: dir, Agent: trackwin.Process{Title: "agent", Command: "sleep 60"}})
	if err != nil {
		t.Fatal(err)
	}

	client := exec.Command("tmux", "-L", socket, "-C", "attach", "-t", "=tracks")
	stdin, err := client.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = client.Wait() })
	var shown []string
	for deadline := time.Now().Add(5 * time.Second); len(shown) == 0 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if shown, err = c.ShownWindows("tracks"); err != nil {
			t.Fatal(err)
		}
	}
	if len(shown) != 1 || shown[0] == w.ID {
		t.Fatalf("shown windows %q, want the Tracks window only", shown)
	}

	var sent []string
	n := &Notifier{
		Tmux:     c,
		Session:  "tracks",
		Settings: func() (settings.Settings, error) { return settings.Settings{}, nil },
		MacOS:    func(title, _ string) { sent = append(sent, title) },
		Log:      func(f string, a ...any) { t.Errorf(f, a...) },
	}
	n.Send(tracks.Notice{Track: "a", Name: "rate-bug", Event: settings.NotifyPROpened, Title: "T"})
	if !slices.Equal(sent, []string{"T"}) {
		t.Errorf("sent %q, want the macOS notification", sent)
	}
	flag := func() string {
		out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", w.ID, "#{window_bell_flag}").Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	for deadline := time.Now().Add(5 * time.Second); flag() != "1" && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
	}
	if got := flag(); got != "1" {
		t.Errorf("the track window's bell flag is %q, want it rung", got)
	}

	if err := c.SelectWindow(w.ID); err != nil {
		t.Fatal(err)
	}
	sent = nil
	n.Send(tracks.Notice{Track: "a", Name: "rate-bug", Event: settings.NotifyPROpened, Title: "T"})
	if len(sent) != 0 {
		t.Errorf("with the track's window on screen sent %q, want nothing", sent)
	}
}
