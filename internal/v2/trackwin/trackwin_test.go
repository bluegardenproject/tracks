package trackwin_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
)

func TestOpenAndAddTerminal(t *testing.T) {
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
		Name:       "api-auth",
		Dir:        dir,
		Agent:      trackwin.Process{Title: "agent", Command: "sleep 60"},
		Terminals:  1,
		DevServers: []trackwin.Process{{Title: "web:3000", Command: "sleep 60"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	panes := mustPanes(t, c, w.ID)
	if len(panes) != 3 {
		t.Fatalf("got %d panes, want agent + terminal + dev server", len(panes))
	}
	agent := byRole(panes, trackwin.RoleAgent)
	if agent.Left != 0 || agent.Title != "agent" {
		t.Errorf("agent pane = %+v, want leftmost with title agent", agent)
	}
	width := agent.Width + 1 + byRole(panes, trackwin.RoleTerminal).Width
	if col := byRole(panes, trackwin.RoleTerminal).Width; col < width*25/100 || col > width*35/100 {
		t.Errorf("right column is %d of %d columns, want about %d%%", col, width, trackwin.ColumnPercent)
	}
	if dev := byRole(panes, trackwin.RoleDevServer); dev.Title != "web:3000" {
		t.Errorf("dev-server title = %q", dev.Title)
	}

	if _, err := trackwin.AddTerminal(c, w.ID); err != nil {
		t.Fatal(err)
	}
	panes = mustPanes(t, c, w.ID)
	if len(panes) != 4 {
		t.Fatalf("got %d panes after AddTerminal, want 4", len(panes))
	}
	var heights []int
	for _, p := range panes {
		if p.Role != trackwin.RoleAgent {
			heights = append(heights, p.Height)
		}
	}
	for _, h := range heights {
		if h < heights[0]-1 || h > heights[0]+1 {
			t.Errorf("column heights %v are not even", heights)
			break
		}
	}

	if _, err := trackwin.AddTerminal(c, "tracks:0"); !errors.Is(err, trackwin.ErrNotTrackWindow) {
		t.Errorf("AddTerminal on the Tracks window: err = %v, want ErrNotTrackWindow", err)
	}
}

func mustPanes(t *testing.T, c *tmux.Client, window string) []tmux.Pane {
	t.Helper()
	panes, err := c.ListPanes(window)
	if err != nil {
		t.Fatal(err)
	}
	return panes
}

func byRole(panes []tmux.Pane, role string) tmux.Pane {
	for _, p := range panes {
		if p.Role == role {
			return p
		}
	}
	return tmux.Pane{}
}

func TestRespawn(t *testing.T) {
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
		Track: "t1", Name: "rate-bug", Kind: "plan", Repo: "api", Dir: dir,
		Agent: trackwin.Process{Title: "Claude Code", Command: "sleep 60"},
	})
	if err != nil {
		t.Fatal(err)
	}
	before := mustPanes(t, c, w.ID)

	worktree := t.TempDir()
	got, err := trackwin.Respawn(c, w.ID, trackwin.Spec{
		Track: "t1", Name: "rate-bug", Kind: "work", Repo: "api,web", Dir: worktree,
		Agent: trackwin.Process{Title: "Claude Code", Command: "pwd > " + filepath.Join(dir, "pwd") + "; sleep 60"},
	})
	if err != nil {
		t.Fatal(err)
	}
	after := mustPanes(t, c, w.ID)
	if got.ID != w.ID || got.Agent != w.Agent || len(after) != len(before) {
		t.Errorf("respawned %+v with panes %+v; want the same window and agent pane", got, after)
	}
	var ran []byte
	for range 50 {
		if ran, err = os.ReadFile(filepath.Join(dir, "pwd")); err == nil && len(ran) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if real, _ := filepath.EvalSymlinks(worktree); strings.TrimSpace(string(ran)) != real && strings.TrimSpace(string(ran)) != worktree {
		t.Errorf("the agent started in %q, want %s", ran, worktree)
	}
	infos, err := trackwin.List(c, "tracks")
	if err != nil || len(infos) != 1 {
		t.Fatalf("windows %+v, %v", infos, err)
	}
	if in := infos[0]; in.Kind != "work" || in.Repo != "api,web" || in.Dir != worktree || in.Track != "t1" {
		t.Errorf("window is %+v; want it on the work track's worktree", in)
	}
}
