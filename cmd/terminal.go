package cmd

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	c := &cobra.Command{
		Use:   "terminal",
		Short: "open a terminal pane in a track's worktree",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			trackID, _ := c.Flags().GetString("track")
			id, err := resolveTrackID(trackID, "tracks terminal")
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			result, err := daemon.NewClient(cfg).OpenTerminal(id)
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			if result.Opened {
				fmt.Println("terminal opened in the track's worktree")
			} else {
				fmt.Println("terminal already open")
			}
			return nil
		},
	}
	c.Flags().String("track", "", "track ID (defaults to $TRACKS_ID)")
	register(c)
}
