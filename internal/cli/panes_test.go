package cli

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/procs"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

func TestPickPanes(t *testing.T) {
	column := []tmux.Pane{{ID: "%3"}, {ID: "%7"}, {ID: "%12"}}
	got, err := pickPanes(column, []string{"12", "%3", "12"})
	if err != nil || len(got) != 2 || got[0].ID != "%3" || got[1].ID != "%12" {
		t.Errorf("pickPanes = %v, %v; want %%3 and %%12 once each, in column order", got, err)
	}
	if got, _ := pickPanes(column, nil); len(got) != 3 {
		t.Errorf("no names picked %v, want every pane", got)
	}
	for _, name := range []string{"1", "agent", ""} {
		if _, err := pickPanes(column, []string{name}); err == nil {
			t.Errorf("pickPanes(%q) found a pane", name)
		}
	}
}

func TestPaneActivity(t *testing.T) {
	snap := procs.New(map[int]procs.Proc{
		10: {PID: 10, PPID: 1, Command: "-zsh"},
		11: {PID: 11, PPID: 10, Command: "pnpm dev"},
		12: {PID: 12, PPID: 11, Command: "node vite"},
		20: {PID: 20, PPID: 1, Command: "-zsh"},
		30: {PID: 30, PPID: 1, Command: "sh -c run"},
	}, map[int][]int{12: {5173}})
	for _, tc := range []struct {
		pane tmux.Pane
		want string
	}{
		{tmux.Pane{Role: trackwin.RoleTerminal, PID: 10}, "pnpm dev; listening on 5173"},
		{tmux.Pane{Role: trackwin.RoleTerminal, PID: 20}, "idle shell"},
		{tmux.Pane{Role: trackwin.RoleDevServer, PID: 30, State: "exited 1"}, "stopped (exited 1)"},
		{tmux.Pane{Role: trackwin.RoleDevServer, PID: 30}, "running"},
		{tmux.Pane{Role: trackwin.RoleSetup, PID: 30, State: "running"}, "setup running"},
		{tmux.Pane{Role: trackwin.RoleSetup, PID: 30, State: "failed 2"}, "setup failed 2"},
	} {
		if got := paneActivity(tc.pane, snap); got != tc.want {
			t.Errorf("paneActivity(%+v) = %q, want %q", tc.pane, got, tc.want)
		}
	}
}

func TestClosePanes(t *testing.T) {
	c := tmux.New(tmuxtest.Socket(t))
	dir := t.TempDir()
	conf := filepath.Join(dir, "tmux.conf")
	if err := (tmux.Conf{DefaultTerminal: "screen-256color", Command: "true"}).Write(conf); err != nil {
		t.Fatal(err)
	}
	if err := c.NewSession(conf, "tracks", "Tracks", "sleep 60"); err != nil {
		t.Fatal(err)
	}
	if _, err := trackwin.Open(c, "tracks", trackwin.Spec{
		Track: "t1", Name: "rate-bug", Dir: dir, Terminals: 2,
		Agent: trackwin.Process{Title: "agent", Command: "sleep 60"},
	}); err != nil {
		t.Fatal(err)
	}
	w, err := trackWindow(c, "tracks", "t1")
	if err != nil {
		t.Fatal(err)
	}
	column, err := trackwin.Column(c, w.Window)
	if err != nil || len(column) != 2 {
		t.Fatalf("column %v: %v", column, err)
	}
	busy, idle := column[0], column[1]
	// With job control, as in a terminal, a job runs in a process group
	// of its own.
	if err := c.RespawnPane(busy.ID, dir, "sh -c 'set -m; sleep 300 & wait'"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		panes, _ := c.ListPanes(busy.ID)
		for _, p := range panes {
			if p.ID == busy.ID {
				busy = p // respawning opens a new terminal
			}
		}
		return slices.Contains(ttyCommands(busy.TTY), "sleep 300")
	})

	var out bytes.Buffer
	if err := closePanes(context.Background(), &out, c, w, []tmux.Pane{busy}); err != nil {
		t.Fatalf("closePanes: %v\n%s", err, out.String())
	}
	if want := "Closed " + paneNumber(busy) + " (terminal).\n"; out.String() != want {
		t.Errorf("printed %q, want %q", out.String(), want)
	}
	left, err := trackwin.Column(c, w.Window)
	if err != nil || len(left) != 1 || left[0].ID != idle.ID {
		t.Errorf("column after closing %v: %v, want only %s", left, err, idle.ID)
	}
	waitFor(t, func() bool { return len(ttyCommands(busy.TTY)) == 0 })
}

// ttyCommands are the command lines of the processes on tty.
func ttyCommands(tty string) []string {
	out, _ := exec.Command("ps", "-t", strings.TrimPrefix(tty, "/dev/"), "-o", "command=").Output()
	var cmds []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			cmds = append(cmds, line)
		}
	}
	return cmds
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatal("timed out")
}
