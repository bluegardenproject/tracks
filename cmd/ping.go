package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/dlog"
	"github.com/spf13/cobra"
)

func init() {
	register(&cobra.Command{
		Use:   "ping",
		Short: "check whether the tracks daemon is reachable",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			cl := daemon.NewClient(cfg)
			r, err := cl.Ping()
			if err != nil {
				return fmt.Errorf("daemon unreachable: %w", err)
			}
			fmt.Printf("daemon %s (pid %d) reachable\n", r.Version, r.PID)
			// The daemon's own stderr goes nowhere (it runs under
			// `tmux run-shell -b`), so point the user at the file instead —
			// this is the command they reach for when something looks wrong.
			if dir, err := cfg.ResolveStateDir(); err == nil {
				fmt.Printf("log: %s\n", filepath.Join(dir, "logs", dlog.FileName))
			}
			return nil
		},
	})
}
