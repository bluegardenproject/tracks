package trackwin_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
)

func TestDestination(t *testing.T) {
	windows := []int{0, 1, 2, 3}
	tests := []struct {
		current int
		move    string
		want    int
		ok      bool
	}{
		{0, trackwin.MoveNext, 1, true},
		{0, trackwin.MovePrev, 3, true},
		{1, trackwin.MoveNext, 2, true},
		{2, trackwin.MovePrev, 1, true},
		{3, trackwin.MoveNext, 0, false},
		{1, trackwin.MovePrev, 0, false},
		{2, trackwin.MoveFirst, 1, true},
		{2, trackwin.MoveLast, 3, true},
		{0, "2", 2, true},
		{2, "2", 2, true},
		{1, "0", 0, true},
		{1, "7", 0, false},
		{1, "sideways", 0, false},
	}
	for _, tt := range tests {
		got, ok := trackwin.Destination(windows, tt.current, tt.move)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Destination from %d, %q = %d, %v; want %d, %v", tt.current, tt.move, got, ok, tt.want, tt.ok)
		}
	}
}

func TestDestinationWithoutTracks(t *testing.T) {
	for _, move := range []string{trackwin.MoveFirst, trackwin.MovePrev, trackwin.MoveNext, trackwin.MoveLast, "1"} {
		if got, ok := trackwin.Destination([]int{0}, 0, move); ok {
			t.Errorf("Destination(%q) with no tracks = %d, want nowhere", move, got)
		}
	}
}

func TestSwitchLandsOnAgent(t *testing.T) {
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
	var agents []string
	for _, name := range []string{"one", "two"} {
		w, err := trackwin.Open(c, "tracks", trackwin.Spec{
			Name: name, Dir: dir, Terminals: 1,
			Agent: trackwin.Process{Title: "agent", Command: "sleep 60"},
		})
		if err != nil {
			t.Fatal(err)
		}
		agents = append(agents, w.Agent)
		// Leave the track with its terminal active.
		if err := c.SelectPane(byRole(mustPanes(t, c, w.ID), trackwin.RoleTerminal).ID); err != nil {
			t.Fatal(err)
		}
	}

	for _, step := range []struct {
		move    string
		current int
		window  string
		pane    string
	}{
		{trackwin.MoveNext, 0, "1", agents[0]},
		{trackwin.MoveLast, 1, "2", agents[1]},
		{"0", 2, "0", ""},
	} {
		if err := trackwin.Switch(c, "tracks", step.move, step.current); err != nil {
			t.Fatal(err)
		}
		window, pane := active(t, socket)
		if window != step.window || (step.pane != "" && pane != step.pane) {
			t.Errorf("after %s: window %s pane %s, want window %s pane %s", step.move, window, pane, step.window, step.pane)
		}
	}
}

func active(t *testing.T, socket string) (window, pane string) {
	t.Helper()
	out, err := exec.Command("tmux", "-L", socket, "display-message", "-p", "-t", "tracks:", "#{window_index} #{pane_id}").Output()
	if err != nil {
		t.Fatal(err)
	}
	window, pane, _ = strings.Cut(strings.TrimSpace(string(out)), " ")
	return window, pane
}
