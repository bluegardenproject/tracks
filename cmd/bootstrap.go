package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

// bootstrap runs when the user invokes `tracks` with no subcommand.
// Order:
//
//  1. Verify tmux is installed.
//  2. Load config.
//  3. Create the tmux session if it doesn't exist (one window
//     called "console" running an interactive shell, since the
//     proper REPL window is deferred to v2).
//  4. Ensure the daemon is up; spawn it as a tmux-server child if
//     not.
//  5. Attach the user to the session (or switch-client if already
//     inside tmux).
//
// All steps are idempotent: re-running `tracks` from another
// terminal does the right thing.
func bootstrap(ctx context.Context) error {
	if err := tmux.Available(); err != nil {
		return err
	}
	cfg, _ := config.Load()
	tm := tmux.New()

	if !tm.HasSession(cfg.Tmux.SessionName) {
		startDir, _ := os.UserHomeDir()
		if err := tm.NewSession(cfg.Tmux.SessionName, "console", "", startDir); err != nil {
			return fmt.Errorf("create tmux session: %w", err)
		}
	}

	// Ensure the daemon is up.
	if err := ensureDaemonUp(cfg); err != nil {
		return err
	}

	// Attach / switch-client.
	if tmux.IsInsideTmux() {
		// Already inside tmux — switch the current client to our
		// session instead of starting a nested attach.
		cmd := exec.Command("tmux", "switch-client", "-t", cfg.Tmux.SessionName)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("tmux switch-client: %w: %s", err, string(out))
		}
		return nil
	}
	return tm.Attach(cfg.Tmux.SessionName)
}

// ensureDaemonUp pings the daemon; if unreachable, spawns it as a
// background child of the tmux server and waits briefly for it to
// come up.
func ensureDaemonUp(cfg config.Config) error {
	cl := daemon.NewClient(cfg)
	cl.DialTimeout = 200 * time.Millisecond
	if _, err := cl.Ping(); err == nil {
		return nil
	}
	self, err := selfBinary()
	if err != nil {
		return fmt.Errorf("find self binary: %w", err)
	}
	// tmux run-shell -b runs as a background child of the tmux
	// server, surviving session detach/reattach. Liveness is gated
	// by the daemon's own `tmux has-session` poll, which exits when
	// the session disappears.
	cmd := exec.Command("tmux", "run-shell", "-b",
		fmt.Sprintf("%s daemon", shellQuote(self)))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("spawn daemon via tmux run-shell: %w: %s",
			err, string(out))
	}
	// Wait up to 3s for the socket to come up.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := cl.Ping(); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become reachable within 3s")
}
