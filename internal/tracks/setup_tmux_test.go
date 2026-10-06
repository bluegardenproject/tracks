package tracks

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// TestSetupPaneScript runs the setup pane's script in a real tmux: a
// success reports itself and closes the pane, a failure keeps it open
// with its exit code.
func TestSetupPaneScript(t *testing.T) {
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
	// A fake tracks on the pane's PATH records how it was called.
	bin := filepath.Join(dir, "bin")
	called := filepath.Join(dir, "called")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := "#!/bin/sh\necho \"$TRACKS_ID $*\" >> '" + called + "'\n"
	if err := os.WriteFile(filepath.Join(bin, "tracks"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Service{SocketDir: dir, BinDir: bin}
	win := TmuxWindows{Tmux: c, Session: "tracks"}

	if err := win.AddSetup(w.ID, dir, trackwin.Process{Title: "setup · api", Command: s.setupCommand("t1", "api", "touch made && true")}); err != nil {
		t.Fatal(err)
	}
	if err := win.AddSetup(w.ID, dir, trackwin.Process{Title: "setup · web's", Command: s.setupCommand("t1", "web's", "exit 3")}); err != nil {
		t.Fatal(err)
	}

	var panes []tmux.Pane
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if panes, err = win.Panes(w.ID); err != nil {
			t.Fatal(err)
		}
		_, api := setupPane(panes, "api")
		web, _ := setupPane(panes, "web's")
		if !api && web.State == "failed 3" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, api := setupPane(panes, "api"); api {
		t.Error("the pane of the setup that succeeded is still open")
	}
	if web, ok := setupPane(panes, "web's"); !ok || web.State != "failed 3" {
		t.Errorf("the failed setup's pane: %+v, %v; want it open with failed 3; panes %+v", web, ok, panes)
	}
	got, _ := os.ReadFile(called)
	calls := strings.Split(strings.TrimSpace(string(got)), "\n")
	slices.Sort(calls)
	want := []string{"t1 report-exit --kind setup --subject web's --code 3", "t1 setup done --repo api"}
	if !slices.Equal(calls, want) {
		t.Errorf("tracks was called with %q; want the success and the failure reported", calls)
	}
	if _, err := os.Stat(filepath.Join(dir, "made")); err != nil {
		t.Error("the setup didn't run in the worktree")
	}
}
