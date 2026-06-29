package services

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

// Spec is the fully-resolved description of one dev server to launch:
// the command and env have already been templated, and LogPath/Dir are
// absolute. The package is deliberately ignorant of config/state types.
type Spec struct {
	Name    string
	Cmd     string            // shell command (already templated)
	Env     map[string]string // extra env (already templated)
	Dir     string            // working directory (the worktree)
	LogPath string            // file to receive stdout+stderr
}

// Process is a running dev server owned by the daemon. It runs in its
// own process group (Setpgid) so the whole tree — the launching shell,
// the node server, and any workers/watchman it forks — can be torn down
// with a single group signal. PGID is that group id (equal to PID, the
// group leader), and is the authoritative teardown handle.
type Process struct {
	Name    string
	PID     int
	PGID    int
	LogPath string

	cmd     *exec.Cmd
	logf    *os.File
	done    chan struct{}
	exitErr error
}

// Start launches spec via the user's login shell in a fresh process
// group, streaming stdout+stderr to LogPath. Using the login shell
// mirrors how Claude and the deps install run, so PATH includes node /
// pnpm provided by nvm/fnm.
func Start(spec Spec) (*Process, error) {
	if err := os.MkdirAll(filepath.Dir(spec.LogPath), 0o755); err != nil {
		return nil, fmt.Errorf("service %s: log dir: %w", spec.Name, err)
	}
	logf, err := os.OpenFile(spec.LogPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("service %s: open log: %w", spec.Name, err)
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/bash"
	}
	cmd := exec.Command(shell, "-lc", spec.Cmd)
	cmd.Dir = spec.Dir
	cmd.Env = append(os.Environ(), flattenEnv(spec.Env)...)
	cmd.Stdout = logf
	cmd.Stderr = logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		_ = logf.Close()
		return nil, fmt.Errorf("service %s: start: %w", spec.Name, err)
	}
	p := &Process{
		Name:    spec.Name,
		PID:     cmd.Process.Pid,
		PGID:    cmd.Process.Pid, // group leader: pgid == pid
		LogPath: spec.LogPath,
		cmd:     cmd,
		logf:    logf,
		done:    make(chan struct{}),
	}
	// Reap the child so it never becomes a zombie, and close the log
	// when it exits — whether it exits on its own or via Stop.
	go func() {
		p.exitErr = cmd.Wait()
		_ = logf.Close()
		close(p.done)
	}()
	return p, nil
}

// Running reports whether the process has not yet exited.
func (p *Process) Running() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

// Wait blocks until the process exits and returns its exit error (nil on
// a clean exit).
func (p *Process) Wait() error {
	<-p.done
	return p.exitErr
}

// Stop tears the process group down: SIGTERM, wait up to grace for it to
// exit, then SIGKILL the remainder. A zero grace skips straight to
// SIGKILL. Safe to call more than once and on an already-exited process.
func (p *Process) Stop(grace time.Duration) {
	if p == nil || p.PGID <= 0 {
		return
	}
	if grace > 0 {
		signalGroup(p.PGID, syscall.SIGTERM)
		if p.waitDone(grace) {
			return
		}
	}
	signalGroup(p.PGID, syscall.SIGKILL)
	p.waitDone(2 * time.Second)
}

// waitDone blocks until the process exits or timeout elapses; returns
// true if it exited.
func (p *Process) waitDone(timeout time.Duration) bool {
	select {
	case <-p.done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// signalGroup sends sig to the whole process group led by pgid
// (kill(-pgid, sig)). Mirrors the daemon supervisor's group-kill: a
// negative pid targets every member, so forked workers don't leak.
func signalGroup(pgid int, sig syscall.Signal) {
	if pgid <= 0 {
		return
	}
	if err := syscall.Kill(-pgid, sig); err != nil {
		_ = syscall.Kill(pgid, sig)
	}
}

// flattenEnv turns a map into deterministic KEY=VALUE pairs (sorted so
// the child's environment is reproducible).
func flattenEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(env))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}
