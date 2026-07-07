package cmd

import (
	"fmt"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/spf13/cobra"
)

func init() {
	c := &cobra.Command{
		Use:   "resume <track-id>",
		Short: "resume a finished track's Claude conversation",
		Long: "Re-creates the track's worktree (if it was removed by `tracks done`) and " +
			"spawns Claude with `--resume <session-id>` so the conversation continues " +
			"from where it left off. The track must be in a terminal state (done/error) " +
			"and must have a session ID (all tracks created since session-ID tracking was added).",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			trackID := args[0]
			cl := daemon.NewClient(cfg)

			fmt.Printf("resuming %s...\n\n", lastN(trackID, 15))
			res, err := cl.ResumeWithProgress(trackID, func(msg string) {
				fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
			})
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			fmt.Println()

			tm := tmux.New()
			if tm.HasSession(cfg.Tmux.SessionName) && res.WindowName != "" {
				_ = tm.SelectWindow(cfg.Tmux.SessionName, res.WindowName)
			}
			return nil
		},
	}
	register(c)
}
