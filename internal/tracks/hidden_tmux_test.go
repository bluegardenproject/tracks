package tracks

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// Closing a track's window ends its hidden panes too: a reviewer must
// not outlive its track.
func TestCloseEndsHiddenPanes(t *testing.T) {
	c := tmux.New(tmuxtest.Socket(t))
	dir := t.TempDir()
	conf := filepath.Join(dir, "tmux.conf")
	if err := (tmux.Conf{DefaultTerminal: "screen-256color", Command: "true"}).Write(conf); err != nil {
		t.Fatal(err)
	}
	if err := c.NewSession(conf, "tracks", "Tracks", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	w, err := trackwin.Open(c, "tracks", trackwin.Spec{Track: "t1", Name: "t1", Dir: dir, Agent: trackwin.Process{Title: "agent", Command: "sleep 60"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, track := range []string{"t1", "t2"} {
		if _, err := trackwin.OpenHidden(c, track, trackwin.RoleReviewer, dir, trackwin.Process{Command: "sleep 60"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := (TmuxWindows{Tmux: c, Session: "tracks"}).Close(w.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := trackwin.Hidden(c, "t1", "", trackwin.RoleReviewer); !errors.Is(err, trackwin.ErrNoHidden) {
		t.Errorf("t1's reviewer outlived its track: %v", err)
	}
	if _, _, err := trackwin.Hidden(c, "t2", "", trackwin.RoleReviewer); err != nil {
		t.Errorf("another track's reviewer went too: %v", err)
	}
}
