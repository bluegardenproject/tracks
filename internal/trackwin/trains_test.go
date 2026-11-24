package trackwin_test

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

func TestTrainSession(t *testing.T) {
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

	spec := trackwin.TrainSpec{Track: "t1", Handle: "reviewer-123456", Role: trackwin.RoleReviewer, Dir: dir,
		Process: trackwin.Process{Title: "reviewer", Command: "sleep 60"}, Header: "head #{@tracks_train_badge}"}
	pane, err := trackwin.OpenTrain(c, spec)
	if err != nil {
		t.Fatal(err)
	}
	session := trackwin.TrainSession("t1", "reviewer-123456")
	if !c.HasSession(session) {
		t.Fatalf("no session %s", session)
	}
	if got := mustPanes(t, c, w.ID); len(got) != 1 {
		t.Errorf("the track's window has %d panes, want the agent only", len(got))
	}
	found, err := trackwin.FindTrain(c, "t1", trackwin.RoleReviewer)
	if err != nil || found.ID != pane || found.Session != session || found.Title != "reviewer" || found.Dead {
		t.Errorf("FindTrain = %+v, %v; want pane %s of %s, alive", found, err, pane, session)
	}
	if _, err := trackwin.FindTrain(c, "t2", trackwin.RoleReviewer); !errors.Is(err, trackwin.ErrNoTrain) {
		t.Errorf("another track's train: %v, want ErrNoTrain", err)
	}
	track, handle, err := trackwin.TrainOf(c, "="+session+":")
	if err != nil || track != "t1" || handle != "reviewer-123456" {
		t.Errorf("TrainOf = %q %q %v", track, handle, err)
	}
	if track, err := trackwin.TrackOf(c, w.Agent); err != nil || track != "t1" {
		t.Errorf("TrackOf a track's pane = %q, %v; want t1, as tracks train finds it from a terminal", track, err)
	}
	if track, _, _ := trackwin.TrainOf(c, w.ID); track != "" {
		t.Errorf("a track's window is train %q's", track)
	}
	if attached, err := trackwin.Attached(c, session); err != nil || attached {
		t.Errorf("Attached = %v, %v; want no client", attached, err)
	}
	for option, want := range map[string]string{"status": "on", "status-position": "top", "status-format[0]": spec.Header, "detach-on-destroy": "on"} {
		out, err := exec.Command("tmux", "-L", c.Socket(), "show-options", "-v", "-t", "="+session+":", option).Output()
		if got := strings.TrimSpace(string(out)); err != nil || got != want {
			t.Errorf("%s = %q, %v; want %q", option, got, err, want)
		}
	}
	if err := trackwin.CloseTrains(c, "t1", ""); err != nil {
		t.Fatal(err)
	}
	if c.HasSession(session) {
		t.Error("the train's session outlived CloseTrains")
	}
}

// A train's pane stays after its agent exits, so it's listed as exited
// until it's ended.
func TestTrainPaneStaysAfterExit(t *testing.T) {
	c := tmux.New(tmuxtest.Socket(t))
	dir := t.TempDir()
	conf := filepath.Join(dir, "tmux.conf")
	if err := (tmux.Conf{DefaultTerminal: "screen-256color", Command: "true"}).Write(conf); err != nil {
		t.Fatal(err)
	}
	if err := c.NewSession(conf, "tracks", "Tracks", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	for _, track := range []string{"t1", "t2"} {
		spec := trackwin.TrainSpec{Track: track, Handle: "reviewer-1", Role: trackwin.RoleReviewer, Dir: dir,
			Process: trackwin.Process{Command: "true"}}
		if _, err := trackwin.OpenTrain(c, spec); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		p, err := trackwin.FindTrain(c, "t1", trackwin.RoleReviewer)
		if err != nil {
			t.Fatalf("the train's pane went with its agent: %v", err)
		}
		if p.Dead {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the train's pane isn't dead after its agent exited")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err := trackwin.CloseTrains(c, "t1", trackwin.RoleReviewer); err != nil {
		t.Fatal(err)
	}
	if _, err := trackwin.FindTrain(c, "t1", trackwin.RoleReviewer); !errors.Is(err, trackwin.ErrNoTrain) {
		t.Errorf("after CloseTrains: %v", err)
	}
	all, err := trackwin.Trains(c, "")
	if err != nil || len(all) != 1 || !strings.HasSuffix(all[0].Session, "t2-reviewer-1") {
		t.Errorf("Trains = %+v, %v; want t2's left", all, err)
	}
}
