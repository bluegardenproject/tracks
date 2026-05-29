package cmd

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
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
			return nil
		},
	})
}
