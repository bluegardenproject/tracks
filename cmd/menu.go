package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tui/menu"
	"github.com/bluegardenproject/tracks/internal/tui/newtrack"
	"github.com/spf13/cobra"
)

func init() {
	register(&cobra.Command{
		Use:   "menu",
		Short: "open the overlay menu (bound to <prefix>+<menu_key> inside the tmux session)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			action, err := menu.PickAction()
			if err != nil {
				if errors.Is(err, menu.ErrCancelled) {
					return nil
				}
				return err
			}
			return runMenuAction(cfg, action)
		},
	})
}

func runMenuAction(cfg config.Config, action menu.Action) error {
	cl := daemon.NewClient(cfg)
	tm := tmux.New()

	switch action {
	case menu.ActionNewTrack:
		return runNewTrackFromMenu(cfg)

	case menu.ActionDashboard:
		// Ensure the dashboard window exists, then switch to it.
		return ensureWindowAndSelect(cfg, tm, "dashboard", "dashboard")

	case menu.ActionList:
		// `ls` is a one-shot dump — show it inside the popup so the
		// user can read it before the popup closes.
		tracks, err := cl.Ls()
		if err != nil {
			return err
		}
		if len(tracks) == 0 {
			fmt.Println("no tracks yet")
			waitForKey()
			return nil
		}
		for _, t := range tracks {
			fmt.Printf("  %-15s  %-30s  %-10s\n", lastN(t.ID, 15), t.Branch, t.Status)
		}
		waitForKey()
		return nil

	case menu.ActionAttach:
		t, err := menu.PickTrack(cl, "Attach to which track?", false)
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			return err
		}
		window := windowNameFor(t.ID)
		exists, _ := tm.HasWindow(cfg.Tmux.SessionName, window)
		if !exists {
			self, _ := selfBinary()
			cmdLine := fmt.Sprintf("%s log %s", shellQuote(self), shellQuote(t.ID))
			if err := tm.NewWindow(cfg.Tmux.SessionName, window, cmdLine, "", true); err != nil {
				return err
			}
		}
		return tm.SelectWindow(cfg.Tmux.SessionName, window)

	case menu.ActionDone:
		t, err := menu.PickTrack(cl, "End which track?", true)
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			return err
		}
		if err := cl.Done(t.ID); err != nil {
			return err
		}
		closeTrackWindow(cfg, t.ID)
		fmt.Printf("done: %s\n", t.ID)
		waitForKey()
		return nil

	case menu.ActionKill:
		t, err := menu.PickTrack(cl, "Kill which track?", true)
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			return err
		}
		if err := cl.Kill(t.ID); err != nil {
			return err
		}
		closeTrackWindow(cfg, t.ID)
		fmt.Printf("killed: %s\n", t.ID)
		waitForKey()
		return nil

	case menu.ActionSettings:
		return openConfigInEditor()

	case menu.ActionGC:
		fmt.Println("running tracks gc...")
		if err := runGC(context.Background(), cfg); err != nil {
			return err
		}
		waitForKey()
		return nil

	case menu.ActionQuitSession:
		yes, err := menu.ConfirmQuit(cfg.Tmux.SessionName)
		if err != nil || !yes {
			return nil
		}
		return tm.KillSession(cfg.Tmux.SessionName)
	}
	return nil
}

// runNewTrackFromMenu is a wrapper that runs the same picker as
// `tracks new` from inside the popup, then asks the daemon and
// opens the per-track window.
func runNewTrackFromMenu(cfg config.Config) error {
	params, err := newtrack.Run(cfg)
	if err != nil {
		if errors.Is(err, newtrack.ErrCancelled) {
			return nil
		}
		return err
	}
	cl := daemon.NewClient(cfg)
	res, err := cl.New(params)
	if err != nil {
		return err
	}
	tm := tmux.New()
	if tm.HasSession(cfg.Tmux.SessionName) {
		self, _ := selfBinary()
		window := windowNameFor(res.TrackID)
		cmdLine := fmt.Sprintf("%s log %s", shellQuote(self), shellQuote(res.TrackID))
		_ = tm.NewWindow(cfg.Tmux.SessionName, window, cmdLine, "", true)
		_ = tm.SelectWindow(cfg.Tmux.SessionName, window)
	}
	fmt.Printf("created %s on %s\n", res.TrackID, res.Branch)
	return nil
}

// ensureWindowAndSelect creates the window if missing (running the
// supplied default command) and selects it.
func ensureWindowAndSelect(cfg config.Config, tm *tmux.Client, window, command string) error {
	exists, err := tm.HasWindow(cfg.Tmux.SessionName, window)
	if err != nil {
		return err
	}
	if !exists {
		self, _ := selfBinary()
		full := fmt.Sprintf("%s %s", shellQuote(self), command)
		if err := tm.NewWindow(cfg.Tmux.SessionName, window, full, "", true); err != nil {
			return err
		}
	}
	return tm.SelectWindow(cfg.Tmux.SessionName, window)
}

// openConfigInEditor execs $EDITOR (or vi as fallback) on the user's
// config file. Inside the tmux popup this opens a usable editor.
func openConfigInEditor() error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	path, err := config.Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// waitForKey blocks until the user presses any key. Used after a
// menu action that prints output, so the popup doesn't vanish before
// the user can read it.
func waitForKey() {
	fmt.Print("\npress enter to close…")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
