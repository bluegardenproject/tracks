package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	c := &cobra.Command{
		Use:   "add-repo <repo-name>",
		Short: "add a repo worktree to the current track (called by Claude via skill)",
		Long: "Provisions a new worktree for the configured repo and mounts it onto the running track. " +
			"Discovers the track ID from the $TRACKS_ID environment variable, which the daemon exports " +
			"when it spawns Claude. Outside a track context the command errors.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			trackID := os.Getenv("TRACKS_ID")
			if trackID == "" {
				return errors.New("$TRACKS_ID not set — `tracks add-repo` only works from inside a track")
			}
			cfg, _ := config.Load()
			cl := daemon.NewClient(cfg)
			res, err := cl.AddRepo(daemon.AddRepoParams{TrackID: trackID, RepoName: args[0]})
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			fmt.Printf("added worktree for %s at %s\n", args[0], res.WorktreePath)
			fmt.Println("Claude can now read/write files under that path.")
			return nil
		},
	}
	register(c)
}
