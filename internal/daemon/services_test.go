package daemon

import (
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

func groupAlive(pgid int) bool { return syscall.Kill(-pgid, 0) == nil }

// spawnGroup starts a shell command in its own process group and returns the
// pgid (== leader pid). Used to exercise the PGID-based teardown paths without
// going through tmux (which the real service start now uses).
func spawnGroup(t *testing.T, script string) int {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("spawn group: %v", err)
	}
	pgid := cmd.Process.Pid
	go func() { _ = cmd.Wait() }() // reap so it doesn't linger as a zombie
	return pgid
}

// newServiceTestServer builds a Server backed by a memory store with its
// state dir under a temp dir, so service logs have somewhere to land.
func newServiceTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.Default()
	cfg.Paths.StateDir = t.TempDir()
	return NewServer(cfg, state.NewMemoryStore(), "test")
}

func TestBuildServiceScript(t *testing.T) {
	script := buildServiceScript(
		map[string]string{"PORT": "20001"},
		[]string{"pnpm install"},
		"pnpm dev",
		"/logs/dev.log",
	)

	// Env is exported (single-quoted value) before the sequence runs.
	if !strings.Contains(script, "export PORT='20001';") {
		t.Errorf("env not exported: %q", script)
	}
	// Deps run before the server, short-circuited with &&.
	if !strings.Contains(script, "pnpm install && pnpm dev") {
		t.Errorf("deps/server not ordered with &&: %q", script)
	}
	// Output is teed to the log so `tracks services`/tail can read it.
	if !strings.Contains(script, "tee '/logs/dev.log'") {
		t.Errorf("output not teed to log: %q", script)
	}
	// Falls back to an interactive shell when the sequence exits.
	if !strings.Contains(script, "exec ${SHELL:-/bin/bash} -l") {
		t.Errorf("no interactive fallback shell: %q", script)
	}
}

func TestBuildServiceScriptNoDeps(t *testing.T) {
	script := buildServiceScript(nil, nil, "pnpm dev", "/logs/dev.log")
	if !strings.Contains(script, "{ pnpm dev ; }") {
		t.Errorf("expected bare server command in the sequence: %q", script)
	}
	if strings.Contains(script, "export ") {
		t.Errorf("no env should be exported: %q", script)
	}
}

func TestBuildServicePaneCommandWrapsInLoginShell(t *testing.T) {
	cmd := buildServicePaneCommand(nil, nil, "pnpm dev", "/logs/dev.log")
	if !strings.Contains(cmd, "${SHELL:-/bin/bash} -lc ") {
		t.Errorf("not wrapped in a login shell: %q", cmd)
	}
}

func TestStopPersistedServicesKillsGroup(t *testing.T) {
	// A backgrounded child ensures we're testing a real group kill, not just
	// the leader.
	pgid := spawnGroup(t, "sleep 60 & sleep 60")
	if !groupAlive(pgid) {
		t.Fatal("group should be alive")
	}
	st := state.ServiceState{Name: "svc", Status: state.ServiceRunning, PGID: pgid}

	stopped := stopPersistedServices([]state.ServiceState{st}, true)
	if stopped[0].Status != state.ServiceStopped {
		t.Errorf("status not marked stopped: %v", stopped[0].Status)
	}
	if stopped[0].ExitedAt == nil {
		t.Error("ExitedAt not set")
	}
	time.Sleep(100 * time.Millisecond)
	if groupAlive(pgid) {
		t.Error("service group still alive after stopPersistedServices — leak")
	}
}

func TestTeardownTrackServices(t *testing.T) {
	srv := newServiceTestServer(t)

	// Two real, live groups that must be killed…
	live1 := spawnGroup(t, "sleep 60")
	live2 := spawnGroup(t, "sleep 60")
	tr := state.Track{
		ID:     "trk-shutdown",
		Status: state.StatusRunning,
		Services: []state.ServiceState{
			{Name: "ready", Status: state.ServiceReady, PGID: live1},
			{Name: "running", Status: state.ServiceRunning, PGID: live2},
			{Name: "already-failed", Status: state.ServiceFailed},
			{Name: "already-stopped", Status: state.ServiceStopped},
		},
	}
	if err := srv.store.Put(tr); err != nil {
		t.Fatal(err)
	}

	srv.teardownTrackServices(tr.ID, true)

	got, _ := srv.store.Get(tr.ID)
	byName := map[string]state.ServiceState{}
	for _, sv := range got.Services {
		byName[sv.Name] = sv
	}
	for _, name := range []string{"ready", "running"} {
		sv := byName[name]
		if sv.Status != state.ServiceStopped {
			t.Errorf("%s: expected stopped, got %q", name, sv.Status)
		}
		if sv.ExitedAt == nil {
			t.Errorf("%s: ExitedAt not set", name)
		}
	}
	if byName["already-failed"].Status != state.ServiceFailed {
		t.Errorf("failed service should be untouched, got %q", byName["already-failed"].Status)
	}
	if byName["already-stopped"].ExitedAt != nil {
		t.Error("already-stopped service should not get a fresh ExitedAt")
	}

	// The live groups must actually be dead.
	time.Sleep(100 * time.Millisecond)
	if groupAlive(live1) || groupAlive(live2) {
		t.Error("a service group survived teardownTrackServices — leak")
	}
}

func TestUpsertService(t *testing.T) {
	list := []state.ServiceState{{Name: "a", Port: 1}}
	list = upsertService(list, state.ServiceState{Name: "b", Port: 2})
	if len(list) != 2 {
		t.Fatalf("expected append, got %v", list)
	}
	list = upsertService(list, state.ServiceState{Name: "a", Port: 9})
	if len(list) != 2 || list[0].Port != 9 {
		t.Errorf("expected in-place replace, got %v", list)
	}
}
