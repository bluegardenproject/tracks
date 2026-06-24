package cmd

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
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
			fmt.Printf("track %s killed; branch retained locally\n", args[0])
			return nil
		},
	})
}
