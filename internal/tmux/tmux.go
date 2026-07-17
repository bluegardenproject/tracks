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

// sessionTarget returns the tmux target that unambiguously names the
// session (a trailing colon: "tracks:"). new-window's -t is a
// *target-window*, so a bare "tracks" is matched as a WINDOW first —
// and when the user has other tmux sessions running in parallel, tmux
// resolves it against whichever session is currently active, landing
// the new window in the wrong session (or failing with "index N in
// use" when that session already has a window at the matched index).
// The trailing colon forces session interpretation and next-free-index
// selection within our session, so tracks never touches other sessions.
func sessionTarget(session string) string { return session + ":" }

// newWindowArgs builds the argv for NewWindow. Extracted so the target
// qualification can be unit-tested without invoking tmux.
func newWindowArgs(session, name, command, startDir string, remainOnExit bool) []string {
	args := []string{"new-window", "-t", sessionTarget(session), "-n", name}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	if remainOnExit {
		args = append(args, "-d")
	}
	if command != "" {
		args = append(args, command)
	}
	return args
}

// NewWindow opens a new window in the session. If command is
// non-empty, the window runs that command instead of the default
// shell; when the command exits the window persists if
// remainOnExit is true.
func (Client) NewWindow(session, name, command, startDir string, remainOnExit bool) error {
	cmd := exec.Command("tmux", newWindowArgs(session, name, command, startDir, remainOnExit)...)
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

// newWindowPaneArgs builds the argv for NewWindowReturningPaneID.
// Extracted for the same testability reason as newWindowArgs.
func newWindowPaneArgs(session, name, command, startDir string) []string {
	args := []string{"new-window", "-t", sessionTarget(session), "-n", name,
		"-P", "-F", "#{pane_pid}"}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	if command != "" {
		args = append(args, command)
	}
	return args
}

// NewWindowReturningPaneID opens a new window like NewWindow and
// returns the new pane's pid. We use this for tracks so the daemon
// can watch the live process (which tmux owns) without dropping the
// "interactive TTY inside tmux" property that lets the user type
// to Claude directly.
//
// The window is NOT created detached: tmux selects it immediately so
// it becomes the session's active window. When this is called from
// inside a display-popup, the popup overlay remains visible on top;
// when the popup closes the user lands on the new track window.
// Always uses remain-on-exit=on so the user can read Claude's final
// output even after the agent itself terminates.
func (Client) NewWindowReturningPaneID(session, name, command, startDir string) (int, error) {
	cmd := exec.Command("tmux", newWindowPaneArgs(session, name, command, startDir)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("tmux new-window: %w: %s", err, strings.TrimSpace(string(out)))
	}
	pid := 0
	if _, scanErr := fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &pid); scanErr != nil || pid <= 0 {
		return 0, fmt.Errorf("tmux new-window: could not parse pid from %q", string(out))
	}
	target := session + ":" + name
	// Pin the name: tmux's automatic-rename (on by default) would
	// otherwise replace our slug-derived name with the foreground
	// process name ("node", "sh", …) and the status-bar tab would
	// stop being meaningful — and name-based targeting would break.
	// Set this first, while the window still carries the -n name we
	// just gave it, so the target resolves.
	_ = exec.Command("tmux", "set-window-option", "-t", target, "automatic-rename", "off").Run()
	_ = exec.Command("tmux", "set-window-option", "-t", target, "remain-on-exit", "on").Run()
	return pid, nil
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

// SetCurrentPaneTitle assigns title to the pane the caller is
// running inside. Used by the dashboard to override tmux's default
// "<hostname>" pane-title fallback. No-op when not running inside
// tmux.
func (Client) SetCurrentPaneTitle(title string) error {
	if !IsInsideTmux() {
		return nil
	}
	cmd := exec.Command("tmux", "select-pane", "-T", title)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux select-pane -T: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CapturePane returns the visible content of a pane. Used by the
// daemon's supervisor to detect when Claude is sitting at its TUI
// prompt waiting for user input (the pane stops changing).
//
// "-p" sends the capture to stdout; "-J" joins wrapped lines so a
// long status line doesn't show up as two different snapshots
// across a terminal resize.
func (Client) CapturePane(session, window string) (string, error) {
	cmd := exec.Command("tmux", "capture-pane", "-t", session+":"+window, "-p", "-J")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tmux capture-pane: %w: %s", err, strings.TrimSpace(buf.String()))
	}
	return buf.String(), nil
}

// SplitWindowRight opens a right-hand vertical split in the named window,
// running command in it (from startDir when non-empty), and returns the new
// pane's ID (e.g. "%42") and the pid of the pane's process. The pane occupies
// percent% of the window width. Dev-server panes are built on top of this: the
// pane_pid is the group leader (tmux setsid's each pane), so it doubles as the
// PGID we tear the server's process tree down with.
func (Client) SplitWindowRight(session, window, command, startDir string, percent int) (paneID string, panePID int, err error) {
	target := session + ":" + window
	args := []string{
		"split-window", "-h",
		"-p", fmt.Sprintf("%d", percent),
		"-t", target,
	}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	args = append(args, "-P", "-F", "#{pane_id} #{pane_pid}", command)
	return runSplit(args, "-h")
}

// SplitPaneDown opens a horizontal split below the pane identified by
// paneID (a string like "%42"), running command in the new pane (from
// startDir when non-empty), and returns the new pane's ID and its pid.
// Used to stack additional service panes below the first one in the right
// column.
func (Client) SplitPaneDown(paneID, command, startDir string) (newPaneID string, panePID int, err error) {
	args := []string{
		"split-window", "-v",
		"-t", paneID,
	}
	if startDir != "" {
		args = append(args, "-c", startDir)
	}
	args = append(args, "-P", "-F", "#{pane_id} #{pane_pid}", command)
	return runSplit(args, "-v")
}

// runSplit executes a `tmux split-window` argv whose format is
// "#{pane_id} #{pane_pid}" and parses both back out. dir is only used to
// label the error.
func runSplit(args []string, dir string) (paneID string, panePID int, err error) {
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		return "", 0, fmt.Errorf("tmux split-window %s: %w: %s", dir, err, strings.TrimSpace(string(out)))
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) != 2 {
		return "", 0, fmt.Errorf("tmux split-window %s: could not parse pane id/pid from %q", dir, string(out))
	}
	if _, scanErr := fmt.Sscanf(fields[1], "%d", &panePID); scanErr != nil || panePID <= 0 {
		return "", 0, fmt.Errorf("tmux split-window %s: could not parse pane pid from %q", dir, string(out))
	}
	return fields[0], panePID, nil
}

// KillPane closes a single pane by ID. No-op when the pane doesn't exist.
func (Client) KillPane(paneID string) error {
	cmd := exec.Command("tmux", "kill-pane", "-t", paneID)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Ignore "no such pane" — treat it as already gone.
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "no such pane") || strings.Contains(msg, "can't find pane") {
			return nil
		}
		return fmt.Errorf("tmux kill-pane %s: %w: %s", paneID, err, msg)
	}
	return nil
}

// SetPaneTitle sets the title displayed in the pane border for the pane
// identified by paneID. Used to label log-viewer panes with the service
// name and port so they're identifiable at a glance.
func (Client) SetPaneTitle(paneID, title string) error {
	cmd := exec.Command("tmux", "select-pane", "-t", paneID, "-T", title)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux select-pane -T: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Available reports whether the tmux binary is on PATH.
func Available() error {
	if _, err := exec.LookPath("tmux"); err != nil {
		return errors.New("tmux not found on PATH; install tmux to use `tracks`")
	}
	return nil
}
