package tracks

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// Closing a track's window ends its trains too: a reviewer must not
// outlive its track.
func TestCloseEndsTrains(t *testing.T) {
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
	for _, tr := range [][2]string{{"t1", "reviewer-1"}, {"t1", "reviewer-2"}, {"t2", "reviewer-1"}} {
		spec := trackwin.TrainSpec{Track: tr[0], Handle: tr[1], Role: trackwin.RoleReviewer, Dir: dir,
			Process: trackwin.Process{Command: "sleep 60"}}
		if _, err := trackwin.OpenTrain(c, spec); err != nil {
			t.Fatal(err)
		}
	}
	windows := TmuxWindows{Tmux: c, Session: "tracks"}
	if r, err := windows.Reviewer("t1", "reviewer-1"); err != nil || r.Pane == "" || r.Handle != "reviewer-1" || r.Attached || r.Exited {
		t.Errorf("Reviewer = %+v, %v; want t1's, running, no overlay", r, err)
	}
	if all, err := windows.Reviewers(); err != nil || len(all["t1"]) != 2 || len(all["t2"]) != 1 {
		t.Errorf("Reviewers = %+v, %v; want t1's two and t2's one", all, err)
	}
	if err := windows.CloseReviewer("t1", "reviewer-2"); err != nil {
		t.Fatal(err)
	}
	if r, _ := windows.Reviewer("t1", "reviewer-2"); r.Pane != "" {
		t.Errorf("reviewer-2 outlived CloseReviewer: %+v", r)
	}
	if r, _ := windows.Reviewer("t1", "reviewer-1"); r.Pane == "" {
		t.Error("closing reviewer-2 closed reviewer-1 too")
	}
	if err := windows.Close(w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := trackwin.FindTrain(c, "t1", trackwin.RoleReviewer); !errors.Is(err, trackwin.ErrNoTrain) {
		t.Errorf("t1's reviewer outlived its track: %v", err)
	}
	if _, err := trackwin.FindTrain(c, "t2", trackwin.RoleReviewer); err != nil {
		t.Errorf("another track's reviewer went too: %v", err)
	}
	session := trackwin.TrainSession("t2", "reviewer-1")
	if err := windows.TrainBadge(session, "x"); err != nil {
		t.Errorf("TrainBadge: %v", err)
	}
	out, err := exec.Command("tmux", "-L", c.Socket(), "show-options", "-v", "-t", "="+session+":", trackwin.TrainBadgeOption).Output()
	if got := strings.TrimSpace(string(out)); err != nil || got != "x" {
		t.Errorf("the badge option is %q, %v; want x", got, err)
	}
}
