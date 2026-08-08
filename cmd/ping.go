package cmd

import (
	"fmt"
	"os"
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
			// Only when it exists: the daemon carries on when it can't open
			// its log, and naming a missing file in exactly that case would
			// send the user looking for the wrong thing.
			if dir, err := cfg.ResolveStateDir(); err == nil {
				p := filepath.Join(dir, "logs", dlog.FileName)
				if _, err := os.Stat(p); err == nil {
					fmt.Printf("log: %s\n", p)
				}
			}
			return nil
		},
	})
}
