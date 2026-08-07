package services

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func groupAlive(pgid int) bool {
	return syscall.Kill(-pgid, 0) == nil
}

// groupGoneWithin polls until no member of the process group is left, or
// the timeout expires. Stop returns once the group *leader* has been
// reaped, but a backgrounded grandchild — signalled at the same instant —
// is reparented to init and reaped there a moment later, and
// kill(-pgid, 0) keeps succeeding until it leaves the process table. The
// leak this guards against is a child that never dies, so polling tests
// the invariant without racing the kernel's reaping.
func groupGoneWithin(pgid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !groupAlive(pgid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return !groupAlive(pgid)
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
	// The log should capture stdout shortly after start. How shortly isn't a
	// property of the code under test — Start runs a *login* shell, so every
	// service pays whatever the user's profile costs before the first line
	// is written — so the deadline is generous and only the "never" case
	// fails. The assertion is on the line itself, not on the file being
	// non-empty: stderr shares this file, so profile chatter would satisfy
	// "non-empty" even if the command never ran.
	if got := string(waitForContent(log, 10*time.Second)); !strings.Contains(got, "hello-from-service") {
		t.Errorf("service log = %q, want it to contain the command's stdout", got)
	}
}

// waitForContent polls path until it has content or the timeout expires,
// then returns whatever is there.
func waitForContent(path string, timeout time.Duration) []byte {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if b, _ := os.ReadFile(path); len(b) > 0 {
			return b
		}
		time.Sleep(20 * time.Millisecond)
	}
	b, _ := os.ReadFile(path)
	return b
}

func TestStopKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	// The shell backgrounds a child sleep; killing only the shell would
	// orphan it. Stop must signal the whole group.
	//
	// The marker file makes the fork observable: without it, Stop could run
	// before the login shell got as far as backgrounding anything, leaving
	// no child to leak and passing the test without exercising the group
	// kill at all.
	marker := filepath.Join(dir, "forked")
	p, err := Start(Spec{
		Name:    "grouped",
		Cmd:     "sleep 60 & printf forked > " + marker + "; sleep 60",
		Dir:     dir,
		LogPath: filepath.Join(dir, "g.log"),
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(waitForContent(marker, 10*time.Second)) == 0 {
		t.Fatal("the shell never backgrounded its child; nothing to test")
	}
	pgid := p.PGID
	if !groupAlive(pgid) {
		t.Fatal("group should be alive right after start")
	}
	p.Stop(2 * time.Second)
	if p.Running() {
		t.Error("process still running after Stop")
	}
	if !groupGoneWithin(pgid, 5*time.Second) {
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
