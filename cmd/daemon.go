package cmd

import (
	"fmt"
	"os"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/spf13/cobra"
)

func init() {
	register(&cobra.Command{
		Use:    "daemon",
		Short:  "run the tracks daemon (internal — spawned by `tracks` itself)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintln(os.Stderr, "warning:", err)
			}
			stateDir, err := cfg.ResolveStateDir()
			if err != nil {
				return fmt.Errorf("resolve state dir: %w", err)
			}
			store, err := state.OpenFileStore(stateDir)
			if err != nil {
				return fmt.Errorf("open state store: %w", err)
			}
			server := daemon.NewServer(cfg, store, Version)
			fmt.Fprintf(os.Stderr, "tracks daemon %s starting (pid %d)\n", Version, os.Getpid())
			return server.Start(c.Context())
		},
	})
}
