package trackwin_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// The Platform opens on the window's left, full height, PlatformWidth
// wide, and isn't one of the right column's panes.
func TestPlatform(t *testing.T) {
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
		Name: "api", Dir: dir, Terminals: 1,
		Agent: trackwin.Process{Title: "agent", Command: "sleep 60"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pane, err := trackwin.Platform(c, w.ID); err != nil || pane != "" {
		t.Fatalf("a new window's Platform: %q, %v; want none", pane, err)
	}
	pane, err := trackwin.OpenPlatform(c, w.ID, dir, trackwin.Process{Title: "trains", Command: "sleep 60"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := trackwin.Platform(c, w.ID); err != nil || got != pane {
		t.Errorf("Platform = %q, %v; want %q", got, err, pane)
	}
	panes := mustPanes(t, c, w.ID)
	p, agent := byRole(panes, trackwin.RolePlatform), byRole(panes, trackwin.RoleAgent)
	if p.Left != 0 || p.Width != trackwin.PlatformWidth || p.Height != agent.Height || agent.Left != trackwin.PlatformWidth+1 {
		t.Errorf("Platform %+v beside agent %+v; want it leftmost, %d wide, full height", p, agent, trackwin.PlatformWidth)
	}
	column, err := trackwin.Column(c, w.ID)
	if err != nil || len(column) != 1 || column[0].Role != trackwin.RoleTerminal {
		t.Errorf("the right column is %+v, %v; want the terminal alone", column, err)
	}
	if _, err := trackwin.AddTerminal(c, w.ID); err != nil {
		t.Fatal(err)
	}
	if column, _ := trackwin.Column(c, w.ID); len(column) != 2 {
		t.Errorf("a terminal added beside the Platform: column %+v", column)
	}
}

// The right column's first pane splits the agent pane, not the window:
// the Platform keeps its width.
func TestTheRightColumnLeavesThePlatformItsWidth(t *testing.T) {
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
	w, err := trackwin.Open(c, "tracks", trackwin.Spec{
		Name: "api", Dir: dir, Agent: trackwin.Process{Title: "agent", Command: "sleep 60"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Wide, where a split of the whole window took the most from it.
	if out, err := exec.Command("tmux", "-L", socket, "resize-window", "-t", w.ID, "-x", "200", "-y", "50").CombinedOutput(); err != nil {
		t.Fatalf("resize: %v %s", err, out)
	}
	if _, err := trackwin.OpenPlatform(c, w.ID, dir, trackwin.Process{Title: "trains", Command: "sleep 60"}); err != nil {
		t.Fatal(err)
	}
	if _, err := trackwin.AddTerminal(c, w.ID); err != nil {
		t.Fatal(err)
	}
	panes := mustPanes(t, c, w.ID)
	if p := byRole(panes, trackwin.RolePlatform); p.Width != trackwin.PlatformWidth || p.Left != 0 {
		t.Errorf("the Platform is %+v after the column opened, want %d wide on the left", p, trackwin.PlatformWidth)
	}
	if term := byRole(panes, trackwin.RoleTerminal); term.Left <= byRole(panes, trackwin.RoleAgent).Left {
		t.Errorf("the terminal %+v isn't right of the agent", term)
	}
}
