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
			window := windowNameFor(t.ID)
			session := cfg.Tmux.SessionName
			exists, err := tm.HasWindow(session, window)
			if err != nil {
				return err
			}
			if !exists {
				// Recreate the per-track window if the user closed it.
				self, _ := selfBinary()
				cmdLine := fmt.Sprintf("%s log %s", shellQuote(self), shellQuote(t.ID))
				if err := tm.NewWindow(session, window, cmdLine, "", true); err != nil {
					return fmt.Errorf("recreate track window: %w", err)
				}
			}
			return tm.SelectWindow(session, window)
		},
	})
}

// windowNameFor returns the tmux window name we use per track. We
// keep it short by taking the trailing 6 hex characters of the ID,
// prefixed with "t-".
func windowNameFor(trackID string) string {
	if len(trackID) >= 6 {
		return "t-" + trackID[len(trackID)-6:]
	}
	return "t-" + trackID
}
