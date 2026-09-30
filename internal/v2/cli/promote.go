package cli

import (
	"fmt"
	"os"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/rpc"
	"github.com/spf13/cobra"
)

func newPromoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "promote [track-id]",
		Short: "make an Ask or Plan track a Work track with its own worktrees",
		Long: "Checks the track's repos out on a new branch and restarts its agent there, " +
			"with edits allowed, on the original task and how the read-only phase ended. " +
			"The track is $TRACKS_ID unless one is given.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id := os.Getenv("TRACKS_ID")
			if len(args) == 1 {
				id = args[0]
			}
			if id == "" {
				return errNoTrack("promote")
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			client := rpc.Client{Socket: paths.Socket}
			r, err := client.Promote(c.Context(), rpc.PromoteParams{ID: id}, func(step string) {
				fmt.Fprintln(c.ErrOrStderr(), step)
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Promoted %s: it has its own worktree now.\n", r.Name)
			return err
		},
	}
}
