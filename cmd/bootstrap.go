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
		// The dashboard is the landing window: it shows every track
		// at a glance and is where most users will live. We invoke
		// `tracks dashboard` directly so the bubbletea TUI owns the
		// pane — no intermediate shell.
		self, err := selfBinary()
		if err != nil {
			return fmt.Errorf("find self binary: %w", err)
		}
		dashboardCmd := fmt.Sprintf("%s dashboard", shellQuote(self))
		if err := tm.NewSession(cfg.Tmux.SessionName, "Dashboard", dashboardCmd, ""); err != nil {
			return fmt.Errorf("create tmux session: %w", err)
		}
		// Bind <prefix><menu_key> globally on this tmux server.
		// `display-popup -E` runs the command in a centered overlay
		// and dismisses on exit. 80%/80% gives the nested huh
		// pickers room to breathe.
		if err := configureMenuKey(cfg, self); err != nil {
			return fmt.Errorf("bind menu key: %w", err)
		}
		// Friendly hint at the bottom-right of every window.
		_ = setStatusHint(cfg)
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

// configureMenuKey binds <prefix><menu_key> to open the tracks menu
// popup. This is a global tmux binding (tmux doesn't support
// per-session keybindings cleanly), so we deliberately pick a key
// that's unbound by default ("t" by default; configurable).
func configureMenuKey(cfg config.Config, selfPath string) error {
	popupCmd := fmt.Sprintf("display-popup -E -w 80%% -h 80%% %s menu",
		shellQuote(selfPath))
	cmd := exec.Command("tmux", "bind-key", cfg.Tmux.MenuKey, popupCmd)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux bind-key: %w: %s", err, string(out))
	}
	return nil
}

// setStatusHint adds a status-right hint reminding the user of the
// menu keybind. Scoped to our session so we don't pollute the user's
// other tmux sessions.
func setStatusHint(cfg config.Config) error {
	hint := fmt.Sprintf("#[fg=cyan]<prefix>+%s menu  #[default]%%H ", cfg.Tmux.MenuKey)
	cmd := exec.Command("tmux", "set-option", "-t", cfg.Tmux.SessionName, "status-right", hint)
	return cmd.Run()
}

// ensureDaemonUp pings the daemon; if unreachable, spawns it as a
// background child of the tmux server and waits briefly for it to
// come up. If the daemon is reachable but its version doesn't
// match the CLI's, automatically restart it — otherwise stale
// daemons silently reject new protocol fields and the user can't
// tell what's wrong.
func ensureDaemonUp(cfg config.Config) error {
	cl := daemon.NewClient(cfg)
	cl.DialTimeout = 200 * time.Millisecond
	if r, err := cl.Ping(); err == nil {
		if r.Version == Version {
			return nil
		}
		fmt.Fprintf(os.Stderr,
			"tracks: daemon is %s, CLI is %s — restarting daemon to match.\n",
			r.Version, Version)
		_ = cl.Shutdown()
		// Wait briefly for the socket to actually go away.
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := cl.Ping(); err != nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return spawnDaemon(cl)
}

// spawnDaemon launches the daemon as a tmux-server background child
// and waits up to 3s for the socket to come up.
func spawnDaemon(cl *daemon.Client) error {
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
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := cl.Ping(); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("daemon did not become reachable within 3s")
}
