package trackwin_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

func TestHiddenPane(t *testing.T) {
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
		Track: "t1", Name: "rate-bug", Dir: dir, Terminals: 1,
		Agent: trackwin.Process{Title: "agent", Command: "sleep 60"},
	})
	if err != nil {
		t.Fatal(err)
	}

	pane, err := trackwin.OpenHidden(c, "t1", trackwin.RoleReviewer, dir, trackwin.Process{Title: "reviewer", Command: "sleep 60"})
	if err != nil {
		t.Fatal(err)
	}
	if got := mustPanes(t, c, w.ID); len(got) != 2 {
		t.Errorf("the track's window has %d panes, want the agent and terminal only while the reviewer is hidden", len(got))
	}
	windows, err := c.ListWindows("tracks")
	if err != nil || len(windows) != 2 {
		t.Errorf("the tracks session has %d windows, %v; the hidden pane must not add one", len(windows), err)
	}
	found, shown, err := trackwin.Hidden(c, "t1", w.ID, trackwin.RoleReviewer)
	if err != nil || found.ID != pane || shown || found.Title != "reviewer" {
		t.Fatalf("Hidden = %+v, shown %v, %v; want the hidden reviewer", found, shown, err)
	}

	if err := trackwin.Show(c, w.ID, pane); err != nil {
		t.Fatal(err)
	}
	panes := mustPanes(t, c, w.ID)
	if len(panes) != 3 || byRole(panes, trackwin.RoleReviewer).ID != pane {
		t.Fatalf("panes %+v; want the reviewer in the track's column", panes)
	}
	if term, rev := byRole(panes, trackwin.RoleTerminal), byRole(panes, trackwin.RoleReviewer); rev.Left != term.Left || rev.Top <= term.Top {
		t.Errorf("reviewer %+v, terminal %+v; want the reviewer under the terminal", rev, term)
	}
	if _, shown, _ := trackwin.Hidden(c, "t1", w.ID, trackwin.RoleReviewer); !shown {
		t.Error("a shown reviewer isn't found as shown")
	}
	if !c.HasSession(trackwin.BackSession) {
		t.Error("the session for hidden panes ended when its only hidden pane was shown")
	}

	if err := trackwin.Hide(c, w.ID, pane); err != nil {
		t.Fatal(err)
	}
	if got := mustPanes(t, c, w.ID); len(got) != 2 {
		t.Errorf("the track's window has %d panes after hiding, want 2", len(got))
	}
	if found, shown, _ := trackwin.Hidden(c, "t1", w.ID, trackwin.RoleReviewer); found.ID != pane || shown {
		t.Errorf("hiding lost the reviewer, or it isn't hidden: %+v, %v", found, shown)
	}

	if _, _, err := trackwin.Hidden(c, "t2", w.ID, trackwin.RoleReviewer); !errors.Is(err, trackwin.ErrNoHidden) {
		t.Errorf("another track's reviewer: %v, want ErrNoHidden", err)
	}
	if err := trackwin.CloseHidden(c, "t1", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := trackwin.Hidden(c, "t1", w.ID, trackwin.RoleReviewer); !errors.Is(err, trackwin.ErrNoHidden) {
		t.Errorf("after CloseHidden: %v, want ErrNoHidden", err)
	}
}
