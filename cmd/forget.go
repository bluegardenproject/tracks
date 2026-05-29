package cmd

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/spf13/cobra"
)

var forgetAllCompleted bool

func init() {
	c := &cobra.Command{
		Use:   "forget [track-id]",
		Short: "remove a finished track from the dashboard (use --completed to clear all)",
		Long: "Removes a track's entry from persistent state so the dashboard stops showing it. " +
			"The track must already be in a terminal status (done/errored) — `tracks forget` will refuse to drop a running track. " +
			"With --completed, every terminal-status track is removed in one go. " +
			"Worktrees are already gone by this point; branches and log files stay on disk.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			cl := daemon.NewClient(cfg)
			if forgetAllCompleted {
				if len(args) > 0 {
					return fmt.Errorf("--completed cannot be combined with a track ID")
				}
				n, err := cl.PruneCompleted()
				if err != nil {
					return fmt.Errorf("daemon: %w", err)
				}
				fmt.Printf("cleared %d completed track(s)\n", n)
				return nil
			}
			if len(args) != 1 {
				return fmt.Errorf("track ID required (or use --completed)")
			}
			if err := cl.Forget(args[0]); err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			fmt.Printf("forgot %s\n", args[0])
			return nil
		},
	}
	c.Flags().BoolVar(&forgetAllCompleted, "completed", false, "drop every done/errored track")
	register(c)
}
