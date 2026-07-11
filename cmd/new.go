package cmd

import (
	"errors"
	"fmt"
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

			cl := daemon.NewClient(cfg)
			result, err := newtrack.Run(cfg, cl)
			if err != nil {
				if errors.Is(err, newtrack.ErrCancelled) {
					fmt.Println("cancelled.")
					return nil
				}
				return err
			}

			tm := tmux.New()

			if result.ResumeID != "" {
				fmt.Println("resuming track...")
				fmt.Println()
				res, err := cl.ResumeWithProgress(result.ResumeID, func(msg string) {
					fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
				})
				if err != nil {
					return fmt.Errorf("daemon: %w", err)
				}
				fmt.Println()
				if tm.HasSession(cfg.Tmux.SessionName) && res.WindowName != "" {
					_ = tm.SelectWindow(cfg.Tmux.SessionName, res.WindowName)
				}
				return nil
			}

			fmt.Println("creating track...")
			fmt.Println()
			res, err := cl.NewWithProgress(result.Params, func(msg string) {
				fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
			})
			if err != nil {
				fmt.Println()
				fmt.Println("daemon:", err)
				fmt.Println()
				// The daemon returns the failed track's ID even on error;
				// offer to save what was entered as a draft or dismiss it.
				if res.TrackID != "" {
					return handleCreateFailure(cl, res.TrackID, err)
				}
				return fmt.Errorf("daemon: %w", err)
			}
			fmt.Println()
			fmt.Println("New track:")
			fmt.Printf("  id:     %s\n", res.TrackID)
			fmt.Printf("  repos:  %s\n", strings.Join(result.Params.Repos, ", "))
			fmt.Printf("  branch: %s\n", res.Branch)
			fmt.Println()

			// The daemon has already opened the per-track tmux
			// window with claude inside. Just switch to it.
			if tm.HasSession(cfg.Tmux.SessionName) && res.WindowName != "" {
				_ = tm.SelectWindow(cfg.Tmux.SessionName, res.WindowName)
			}
			return nil
		},
	}
	register(c)
}
