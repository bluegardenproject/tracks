// Package tmux runs tmux commands against one tmux server, selected by
// socket name (`tmux -L`). Tracks v2 never talks to the user's default
// tmux server.
package tmux

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// TestSocketPrefix is the only socket prefix allowed inside a test
// binary, so no test can reach a real Tracks server.
const TestSocketPrefix = "tracks-test-"

// Client runs tmux commands on one server.
type Client struct{ socket string }

// New returns a client for the server on socket.
func New(socket string) *Client { return &Client{socket: socket} }

// Socket is the server's socket name.
func (c *Client) Socket() string { return c.socket }

func (c *Client) args(args ...string) []string {
	if testing.Testing() && !strings.HasPrefix(c.socket, TestSocketPrefix) {
		panic(fmt.Sprintf("tmux: test used socket %q; use tmuxtest.Socket", c.socket))
	}
	return append([]string{"-L", c.socket}, args...)
}

// run executes one tmux command and returns its trimmed stdout.
func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command("tmux", c.args(args...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tmux %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// HasSession reports whether the server runs a session called name.
func (c *Client) HasSession(name string) bool {
	return exec.Command("tmux", c.args("has-session", "-t", "="+name)...).Run() == nil
}

// NewSession starts the server with configFile, unless it's already
// running, and creates a detached session whose first window is
// called window and runs command.
func (c *Client) NewSession(configFile, name, window, command string) error {
	args := []string{"-f", configFile, "new-session", "-d", "-s", name, "-n", window}
	if command != "" {
		args = append(args, command)
	}
	_, err := c.run(args...)
	return err
}

// SelectWindow makes window current in its session. window is an ID
// ("@3") or "session:index".
func (c *Client) SelectWindow(window string) error {
	_, err := c.run("select-window", "-t", window)
	return err
}

// Window is one window of a session.
type Window struct {
	ID    string
	Index int
	Name  string
	// Kind, Repo and Dir are the @tracks_kind, @tracks_repo and
	// @tracks_dir options of a track's window.
	Kind, Repo, Dir string
}

// ListWindows returns session's windows, in order.
func (c *Client) ListWindows(session string) ([]Window, error) {
	out, err := c.run("list-windows", "-t", "="+session, "-F",
		"#{window_id}\t#{window_index}\t#{@tracks_kind}\t#{@tracks_repo}\t#{@tracks_dir}\t#{window_name}")
	if err != nil {
		return nil, err
	}
	var windows []Window
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 6)
		if len(f) != 6 {
			continue
		}
		index, err := strconv.Atoi(f[1])
		if err != nil {
			return nil, fmt.Errorf("window index %q: %w", f[1], err)
		}
		windows = append(windows, Window{ID: f[0], Index: index, Kind: f[2], Repo: f[3], Dir: f[4], Name: f[5]})
	}
	return windows, nil
}

// KillWindow closes window and ends the processes in its panes.
func (c *Client) KillWindow(window string) error {
	_, err := c.run("kill-window", "-t", window)
	return err
}

// KillServer stops the server and everything in it. Not running is
// not an error.
func (c *Client) KillServer() error {
	if _, err := c.run("kill-server"); err != nil && c.running() {
		return err
	}
	return nil
}

func (c *Client) running() bool {
	return exec.Command("tmux", c.args("list-sessions")...).Run() == nil
}

// Attach replaces the current process with a tmux client attached to
// session. It only returns on error.
func (c *Client) Attach(session string) error {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not on PATH: %w", err)
	}
	return syscall.Exec(bin, append([]string{"tmux"}, c.args("attach-session", "-t", "="+session)...), os.Environ())
}
