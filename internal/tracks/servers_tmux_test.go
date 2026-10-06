package tracks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tmux/tmuxtest"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// TestServerPanes runs dev servers in a real tmux: the port reaches the
// command, a crash keeps its pane with the code, and stopping ends the
// server's children too.
func TestServerPanes(t *testing.T) {
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
	// Its setup --wait fails, as a failed setup would.
	fake := "#!/bin/sh\necho \"$TRACKS_ID $*\" >> '" + called + "'\n[ \"$1 $2\" = \"setup --wait\" ] && exit 3\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "tracks"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Service{SocketDir: dir, BinDir: bin}
	win := TmuxWindows{Tmux: c, Session: "tracks"}
	start := func(name, command string, port int, setup bool) {
		t.Helper()
		d := devServer{repo: track.Repo{Name: "api"}, def: store.DevServer{Name: name, Command: command}, setup: setup}
		if err := win.AddDevServer(w.ID, dir, trackwin.Process{Title: name, Command: s.serverCommand("t1", d, port)}, d.key(), port); err != nil {
			t.Fatal(err)
		}
	}
	start("web", "echo port=$PORT tpl={{port}}; sleep 300 & sleep 300", 20005, false)
	start("bad", "exit 4", 0, false)
	start("late", "touch never-ran", 0, true)

	var web, bad, late tmux.Pane
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		panes, err := win.Panes(w.ID)
		if err != nil {
			t.Fatal(err)
		}
		web, _ = serverPane(panes, "api/web")
		bad, _ = serverPane(panes, "api/bad")
		late, _ = serverPane(panes, "api/late")
		out, _ := win.Capture(web.ID, 50)
		reported, _ := os.ReadFile(called)
		if bad.State == "exited 4" && late.State == "exited 3" && strings.Contains(out, "port=20005 tpl=20005") &&
			strings.Contains(string(reported), "report-exit") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if out, _ := win.Capture(web.ID, 50); !strings.Contains(out, "port=20005 tpl=20005") || web.Port != 20005 {
		t.Errorf("web's pane (port %d) shows %q; want the port in $PORT and {{port}}", web.Port, out)
	}
	if serverState(bad) != ServerCrashed || exitCode(bad.State) != 4 {
		t.Errorf("bad's state %q; want crashed with 4", bad.State)
	}
	got, _ := os.ReadFile(called)
	if !strings.Contains(string(got), "t1 report-exit --kind server --subject api/bad --code 4") || strings.Contains(string(got), "api/late") {
		t.Errorf("tracks was called with %q; want bad's crash reported, and nothing for late, whose setup failed", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "never-ran")); err == nil {
		t.Error("late started although its setup failed")
	}
	if out, _ := win.Capture(late.ID, 20); !strings.Contains(out, "late didn't start: the setup didn't finish.") {
		t.Errorf("late's pane shows %q", out)
	}

	if err := win.StopPane(web); err != nil {
		t.Fatal(err)
	}
	if groupAlive(web.PID) {
		out, _ := exec.Command("ps", "-A", "-o", "pid=,pgid=,stat=,command=").Output()
		var group []string
		for _, line := range strings.Split(string(out), "\n") {
			if f := strings.Fields(line); len(f) > 1 && f[1] == strconv.Itoa(web.PID) {
				group = append(group, line)
			}
		}
		t.Errorf("web's process group still has processes after StopPane:\n%s", strings.Join(group, "\n"))
	}
	if err := win.Close(w.ID); err != nil {
		t.Fatal(err)
	}
}

// TestStopGroupKillsWhatIgnoresTERM checks the SIGKILL after the grace.
func TestStopGroupKillsWhatIgnoresTERM(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap '' TERM; sleep 300 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	time.Sleep(100 * time.Millisecond)
	start := time.Now()
	if err := stopGroup(cmd.Process.Pid, 300*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < 300*time.Millisecond {
		t.Errorf("took %v; the group ignores TERM, so it should wait the grace", took)
	}
	time.Sleep(100 * time.Millisecond)
	if groupAlive(cmd.Process.Pid) {
		t.Error("the group survived")
	}
}
