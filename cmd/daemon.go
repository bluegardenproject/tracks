package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/dlog"
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
			// The daemon is started with `tmux run-shell -b`, which throws
			// its stderr away — so without a file sink every diagnostic the
			// daemon emits (reconcile decisions, config-reload failures, gc
			// actions) is lost exactly when it's needed. Non-fatal: a daemon
			// that can't open its log still runs, logging to stderr alone.
			logPath, logErr := dlog.Init(filepath.Join(stateDir, "logs"))
			if logErr != nil {
				fmt.Fprintln(os.Stderr, "warning: could not open daemon log:", logErr)
			}
			defer dlog.Close()
			store, err := state.OpenFileStore(stateDir)
			if err != nil {
				return fmt.Errorf("open state store: %w", err)
			}
			server := daemon.NewServer(cfg, store, Version)
			dlog.Printf("tracks daemon %s starting (pid %d, log %s)", Version, os.Getpid(), logPath)
			return server.Start(c.Context())
		},
	})
}
