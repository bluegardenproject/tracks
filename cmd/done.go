package cmd

import (
	"fmt"
	"os"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/spf13/cobra"
)

func init() {
	register(&cobra.Command{
		Use:   "done <track-id>",
		Short: "end a track gracefully (SIGTERM, remove worktrees, keep branch)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			cl := daemon.NewClient(cfg)
			if err := cl.Done(args[0]); err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			closeTrackWindow(cfg, args[0])
			fmt.Printf("track %s done; branch retained locally\n", args[0])
			return nil
		},
	})

	register(&cobra.Command{
		Use:   "kill <track-id>",
		Short: "end a track with prejudice (SIGKILL, remove worktrees, keep branch)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			cl := daemon.NewClient(cfg)
			if err := cl.Kill(args[0]); err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			closeTrackWindow(cfg, args[0])
			fmt.Printf("track %s killed; branch retained locally\n", args[0])
			return nil
		},
	})
}

// closeTrackWindow is a best-effort cleanup of the per-track tmux
// window. It is a no-op when tmux isn't running. Failures are
// reported on stderr but don't fail the command — the underlying
// daemon work has already succeeded by this point.
func closeTrackWindow(cfg config.Config, trackID string) {
	tm := tmux.New()
	if !tm.HasSession(cfg.Tmux.SessionName) {
		return
	}
	if err := tm.KillWindow(cfg.Tmux.SessionName, windowNameFor(trackID)); err != nil {
		fmt.Fprintf(os.Stderr, "warning: close track window: %v\n", err)
	}
}
