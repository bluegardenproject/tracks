package services

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func groupAlive(pgid int) bool {
	return syscall.Kill(-pgid, 0) == nil
}

func TestStartWritesLogAndRuns(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "svc.log")
	p, err := Start(Spec{
		Name:    "echoer",
		Cmd:     "echo hello-from-service; sleep 30",
		Dir:     dir,
		LogPath: log,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer p.Stop(0)

	if !p.Running() {
		t.Error("expected service to be running")
	}
	// The log should capture stdout shortly after start.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(log)
		if len(b) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	b, _ := os.ReadFile(log)
	if string(b) == "" {
		t.Error("service log is empty; stdout not captured")
	}
}

func TestStopKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	// The shell backgrounds a child sleep; killing only the shell would
	// orphan it. Stop must signal the whole group.
	p, err := Start(Spec{
		Name:    "grouped",
		Cmd:     "sleep 60 & sleep 60",
		Dir:     dir,
		LogPath: filepath.Join(dir, "g.log"),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	pgid := p.PGID
	if !groupAlive(pgid) {
		t.Fatal("group should be alive right after start")
	}
	p.Stop(2 * time.Second)
	if p.Running() {
		t.Error("process still running after Stop")
	}
	if groupAlive(pgid) {
		t.Error("process group still alive after Stop — children leaked")
	}
}

func TestEnvIsPassed(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "env.log")
	p, err := Start(Spec{
		Name:    "envtest",
		Cmd:     "printf '%s' \"$TRACKS_TEST_VAR\"",
		Env:     map[string]string{"TRACKS_TEST_VAR": "wired"},
		Dir:     dir,
		LogPath: log,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	b, _ := os.ReadFile(log)
	if string(b) != "wired" {
		t.Errorf("env not passed: log=%q", string(b))
	}
}

func TestWaitReportsExitError(t *testing.T) {
	dir := t.TempDir()
	p, err := Start(Spec{
		Name:    "failer",
		Cmd:     "exit 7",
		Dir:     dir,
		LogPath: filepath.Join(dir, "f.log"),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Wait(); err == nil {
		t.Error("expected non-nil exit error for exit 7")
	}
	if p.Running() {
		t.Error("Running() should be false after exit")
	}
}
