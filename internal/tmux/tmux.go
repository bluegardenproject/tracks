// Package tmux wraps the small set of tmux operations `tracks` needs.
//
// `tracks` owns exactly one tmux session and opens one window per
// running track plus persistent "console" and "dashboard" windows.
// Tmux is the runtime UI surface; this package keeps every tmux
// shell-out in one place so the rest of the codebase can stay
// terminal-agnostic.
package tmux

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// Client wraps tmux subprocess calls. There's no state — each
// method shells out fresh — but a struct makes the API self-documenting
// and gives tests a clean injection point.
type Client struct{}

// New returns a Client. Trivial constructor for symmetry with the
// other packages.
func New() *Client { return &Client{} }

// HasSession reports whether `tmux has-session -t <name>` exits zero.
// Treats tmux-not-installed as "no session" so callers degrade
// gracefully.
func (Client) HasSession(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

// NewSession creates a new detached session with one initial window.
// Returns nil if the session already exists (idempotent — useful
// when bootstrap commands run twice).
func (Client) NewSession(name, initialWindowName, initialCommand, startDir string) error {
	c := Client{}
	if c.HasSession(name) {
		return nil
	}
	args := []string{"new-session", "-d", "-s", name, "-n", initialWindowName}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	if initialCommand != "" {
		args = append(args, initialCommand)
	}
	cmd := exec.Command("tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-session: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// HasWindow reports whether the named window exists in the session.
// We list windows by format and grep — `tmux has-window` doesn't
// exist as a first-class command.
func (Client) HasWindow(session, window string) (bool, error) {
	cmd := exec.Command("tmux", "list-windows", "-t", session, "-F", "#W")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return false, err
	}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == window {
			return true, nil
		}
	}
	return false, nil
}

// NewWindow opens a new window in the session. If command is
// non-empty, the window runs that command instead of the default
// shell; when the command exits the window persists if
// remainOnExit is true.
func (Client) NewWindow(session, name, command, startDir string, remainOnExit bool) error {
	args := []string{"new-window", "-t", session, "-n", name}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	if remainOnExit {
		args = append(args, "-d")
	}
	if command != "" {
		args = append(args, command)
	}
	cmd := exec.Command("tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-window: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if remainOnExit {
		// Use set-option so the window persists even if the command
		// inside it exits with an error. Useful for the per-track
		// tail window which the user wants to read after the track
		// finishes.
		setOpt := exec.Command("tmux", "set-window-option", "-t", session+":"+name, "remain-on-exit", "on")
		_ = setOpt.Run()
	}
	return nil
}

// KillWindow closes the named window. No-op when missing.
func (Client) KillWindow(session, name string) error {
	exists, err := Client{}.HasWindow(session, name)
	if err != nil || !exists {
		return err
	}
	cmd := exec.Command("tmux", "kill-window", "-t", session+":"+name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux kill-window %s:%s: %w: %s", session, name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// KillSession tears down the entire session. This is what stops the
// daemon (the daemon's tmux-watch loop notices the session is gone
// and exits).
func (Client) KillSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux kill-session: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// SelectWindow switches the active window inside the session.
func (Client) SelectWindow(session, window string) error {
	cmd := exec.Command("tmux", "select-window", "-t", session+":"+window)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux select-window: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Attach replaces the current process with `tmux attach-session -t
// <name>`. Only sensible from a foreground CLI invocation; callers
// inside the daemon should not use this.
//
// Returns only on error — on success this never returns because
// execve replaces the calling process.
func (Client) Attach(name string) error {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not on PATH: %w", err)
	}
	return syscall.Exec(bin, []string{"tmux", "attach-session", "-t", name}, os.Environ())
}

// IsInsideTmux reports whether the current process is already
// running inside a tmux session. Used by the bootstrap command to
// decide between "create + attach" and "create + switch-client".
func IsInsideTmux() bool { return os.Getenv("TMUX") != "" }

// Available reports whether the tmux binary is on PATH.
func Available() error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return errors.New("tmux not found on PATH; install tmux to use `tracks`")
	}
	return nil
}
