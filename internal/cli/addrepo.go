package cli

import (
	"fmt"
	"os"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/spf13/cobra"
)

func newAddRepoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-repo <repo>",
		Short: "give this track a worktree of another repo from the Repositories tab",
		Long: "Checks the repo out on the track's branch, next to its other worktrees, and adds it " +
			"to the track. Run inside a Work track: the track is $TRACKS_ID.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			id := os.Getenv("TRACKS_ID")
			if id == "" {
				return errNoTrack("add-repo")
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			client := rpc.Client{Socket: paths.Socket}
			r, err := client.AddRepo(c.Context(), rpc.AddRepoParams{ID: id, Repo: args[0]}, func(step string) {
				fmt.Fprintln(c.ErrOrStderr(), step)
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Added %s at %s. You can read and write files there.\n", r.Name, r.Worktree)
			return err
		},
	}
}
