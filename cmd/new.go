package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tui/newtrack"
	"github.com/spf13/cobra"
)

func init() {
	c := &cobra.Command{
		Use:   "new",
		Short: "start a new track via the interactive picker",
		Long: "Walks the user through repo selection, branch type, and task prompt, then asks the daemon to create the track. " +
			"The branch slug is auto-derived from the task prompt (Jira ticket + first descriptive words). " +
			"The daemon does the actual worktree provisioning; this command is just the UI.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			params, err := newtrack.Run(cfg)
			if err != nil {
				if errors.Is(err, newtrack.ErrCancelled) {
					fmt.Println("cancelled.")
					return nil
				}
				return err
			}

			cl := daemon.NewClient(cfg)
			fmt.Println("creating track...")
			fmt.Println()
			res, err := cl.NewWithProgress(params, func(msg string) {
				fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
			})
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			fmt.Println()
			fmt.Println("New track:")
			fmt.Printf("  id:     %s\n", res.TrackID)
			fmt.Printf("  repos:  %s\n", strings.Join(params.Repos, ", "))
			fmt.Printf("  branch: %s\n", res.Branch)
			fmt.Println()

			// Open the per-track tmux window if we have a session.
			tm := tmux.New()
			if tm.HasSession(cfg.Tmux.SessionName) {
				self, err := selfBinary()
				if err == nil {
					window := windowNameFor(res.TrackID)
					cmdLine := fmt.Sprintf("%s log %s", shellQuote(self), shellQuote(res.TrackID))
					if err := tm.NewWindow(cfg.Tmux.SessionName, window, cmdLine, "", true); err != nil {
						fmt.Fprintf(os.Stderr, "warning: could not open tmux window: %v\n", err)
					}
				}
			}
			return nil
		},
	}
	register(c)
}
