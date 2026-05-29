package cmd

import (
	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/tui/dashboard"
	"github.com/spf13/cobra"
)

func init() {
	register(&cobra.Command{
		Use:   "dashboard",
		Short: "live dashboard of every track and pending approval",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			return dashboard.Run(cfg)
		},
	})
}
