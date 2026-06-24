package cmd

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/spf13/cobra"
)

func init() {
	register(&cobra.Command{
		Use:   "attach <track-id>",
		Short: "switch to a track's tmux window",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			cl := daemon.NewClient(cfg)
			t, found, err := cl.Get(args[0])
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("track %s not found", args[0])
			}
			tm := tmux.New()
			window := t.WindowName()
			session := cfg.Tmux.SessionName
			exists, err := tm.HasWindow(session, window)
			if err != nil {
				return err
			}
			if !exists {
				return fmt.Errorf("track window %q no longer exists — claude likely exited; the track may be done", window)
			}
			return tm.SelectWindow(session, window)
		},
	})
}
