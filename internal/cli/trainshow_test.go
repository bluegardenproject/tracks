package cli

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

type fakeTrains struct {
	trains []rpc.Train
	ended  []string
}

func (f *fakeTrains) TrainList(context.Context, string) ([]rpc.Train, error) { return f.trains, nil }

func (f *fakeTrains) TrainEnd(_ context.Context, id, name string) error {
	f.ended = append(f.ended, id+" "+name)
	return nil
}

func TestTrainPopup(t *testing.T) {
	th := theme.Default()
	p := trainPopup("tracks", "/dev/ttys001", "tracks-train-t1-reviewer-1", th)
	want := "env -u TMUX tmux -L 'tracks' attach-session -t '=tracks-train-t1-reviewer-1'"
	if p.Command != want || p.Client != "/dev/ttys001" || p.Border != th.Value(theme.OverlayBorder) {
		t.Errorf("popup %+v; want %s over the client, with a border", p, want)
	}
}

// End train in a train's header ends that train once the user confirms;
// a click elsewhere, or Hide without a client, does nothing.
func TestTrainAction(t *testing.T) {
	c := tmux.New(tmuxtest.Socket(t))
	dir := t.TempDir()
	conf := filepath.Join(dir, "tmux.conf")
	if err := (tmux.Conf{DefaultTerminal: "screen-256color", Command: "true"}).Write(conf); err != nil {
		t.Fatal(err)
	}
	if err := c.NewSession(conf, sessionName, "Tracks", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	if _, err := trackwin.OpenTrain(c, trackwin.TrainSpec{Track: "t1", Handle: "reviewer-1", Role: trackwin.RoleReviewer,
		Dir: dir, Process: trackwin.Process{Command: "sleep 60"}}); err != nil {
		t.Fatal(err)
	}
	window := "=" + trackwin.TrainSession("t1", "reviewer-1") + ":"
	daemon := &fakeTrains{}
	var asked []string
	answer := false
	confirmEnd := func(handle string) (bool, error) { asked = append(asked, handle); return answer, nil }
	if msg := trainAction(context.Background(), c, daemon, actions.HideTrainRange, window, "", confirmEnd); msg != "" || len(daemon.ended) != 0 {
		t.Errorf("Hide without a client: %q, ended %v", msg, daemon.ended)
	}
	if msg := trainAction(context.Background(), c, daemon, actions.EndTrainRange, sessionName+":", "", confirmEnd); msg != "" || len(daemon.ended) != 0 || len(asked) != 0 {
		t.Errorf("End train outside a train: %q, ended %v, asked %v", msg, daemon.ended, asked)
	}
	if msg := trainAction(context.Background(), c, daemon, actions.EndTrainRange, window, "/dev/ttys001", confirmEnd); msg != "" || len(daemon.ended) != 0 ||
		len(asked) != 1 || asked[0] != "reviewer-1" {
		t.Errorf("End train, cancelled: %q, ended %v, asked %v; want reviewer-1 asked about and kept", msg, daemon.ended, asked)
	}
	if msg := trainAction(context.Background(), c, daemon, actions.EndTrainRange, window, "/dev/ttys001",
		func(string) (bool, error) { return false, errors.New("no client") }); !strings.Contains(msg, "no client") || len(daemon.ended) != 0 {
		t.Errorf("End train, couldn't ask: %q, ended %v", msg, daemon.ended)
	}
	answer = true
	if msg := trainAction(context.Background(), c, daemon, actions.EndTrainRange, window, "/dev/ttys001", confirmEnd); msg != "" ||
		len(daemon.ended) != 1 || daemon.ended[0] != "t1 reviewer-1" {
		t.Errorf("End train: %q, ended %v; want t1's reviewer-1", msg, daemon.ended)
	}
}

func TestShowTrainResolves(t *testing.T) {
	c := tmux.New(tmuxtest.Socket(t))
	daemon := &fakeTrains{trains: []rpc.Train{{Handle: "reviewer-123456"}}}
	err := showTrain(context.Background(), c, daemon, theme.Default(), "t1", "detective", "/dev/ttys001")
	if err == nil || !strings.Contains(err.Error(), "no train") {
		t.Errorf("a train that isn't there: %v", err)
	}
	err = showTrain(context.Background(), c, daemon, theme.Default(), "t1", "reviewer", "/dev/ttys001")
	if err == nil || !strings.Contains(err.Error(), "reviewer-123456 has no session") {
		t.Errorf("a train without a session, by its prefix: %v", err)
	}
}
