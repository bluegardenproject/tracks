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
		Use:   "promote <track-id>",
		Short: "promote a read-only ask/plan track to a work track",
		Long: "Turns a worktree-less ask/plan track into a work track: creates a branch + worktree " +
			"off base for each repo and re-spawns Claude in it with edit permissions, seeded with the " +
			"original task. A running plan-mode session can't be switched to edit-in-place, so this is a " +
			"re-spawn rather than an in-place change.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cl := daemon.NewClient(cfg)
			fmt.Println("promoting track...")
			fmt.Println()
			res, err := cl.PromoteWithProgress(args[0], func(msg string) {
				fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
			})
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			fmt.Println()
			fmt.Printf("promoted to a work track on branch %s\n", res.Branch)

			tm := tmux.New()
			if tm.HasSession(cfg.Tmux.SessionName) && res.WindowName != "" {
				_ = tm.SelectWindow(cfg.Tmux.SessionName, res.WindowName)
			}
			return nil
		},
	}
	register(c)
}
