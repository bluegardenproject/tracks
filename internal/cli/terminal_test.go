package cli

import (
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

func TestOpenTerminal(t *testing.T) {
	c := tmux.New(tmuxtest.Socket(t))
	dir := t.TempDir()
	conf := filepath.Join(dir, "tmux.conf")
	if err := (tmux.Conf{DefaultTerminal: "screen-256color", Command: "true"}).Write(conf); err != nil {
		t.Fatal(err)
	}
	if err := c.NewSession(conf, "tracks", "Tracks", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	w, err := trackwin.Open(c, "tracks", trackwin.Spec{
		Track: "t1", Name: "rate-bug", Dir: dir,
		Agent: trackwin.Process{Title: "agent", Command: "sleep 60"},
	})
	if err != nil {
		t.Fatal(err)
	}

	name, err := openTerminal(c, "tracks", "t1")
	if err != nil || name != "rate-bug" {
		t.Fatalf("opened in %q: %v", name, err)
	}
	panes, err := c.ListPanes(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	var terminals []tmux.Pane
	for _, p := range panes {
		if p.Role == trackwin.RoleTerminal {
			terminals = append(terminals, p)
		}
	}
	if len(terminals) != 1 {
		t.Errorf("panes %+v, want one terminal", panes)
	}

	if _, err := openTerminal(c, "tracks", "t2"); err == nil || err.Error() != "track t2 has no open window" {
		t.Errorf("a track without a window: %v", err)
	}
}
