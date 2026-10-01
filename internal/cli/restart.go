package cli

import (
	"fmt"
	"os"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/spf13/cobra"
)

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart [track-id]",
		Short: "start a track's agent again once it exited",
		Long: "Restarts the agent in the track's pane, on the same session. " +
			"The track is $TRACKS_ID unless one is given, so running it in the shell " +
			"the agent left behind brings the agent back.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id := os.Getenv("TRACKS_ID")
			if len(args) == 1 {
				id = args[0]
			}
			if id == "" {
				return errNoTrack("restart")
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			client := rpc.Client{Socket: paths.Socket}
			r, err := client.Restart(c.Context(), rpc.RestartParams{ID: id}, func(step string) {
				fmt.Fprintln(c.ErrOrStderr(), step)
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Restarted %s.\n", r.Name)
			return err
		},
	}
}
