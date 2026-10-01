package cli

import (
	"context"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "stop Tracks v2 and its tmux server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			const name = "Tracks v2"
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			server := tmux.New(paths.TmuxSocket)
			if !server.HasSession(sessionName) {
				_, err := fmt.Fprintf(c.OutOrStdout(), "%s isn't running.\n", name)
				return err
			}
			if err := closeTracks(c.Context(), server, paths); err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "%s stopped.\n", name)
			return err
		},
	}
}

// closeTracks closes Tracks: the daemon first, which finishes the
// creations under way, then the tmux server with every window. The
// daemon interrupts the tracks left without a window when it next
// starts.
func closeTracks(ctx context.Context, server *tmux.Client, paths platform.Paths) error {
	if err := stopDaemon(ctx, rpc.Client{Socket: paths.Socket}); err != nil {
		return err
	}
	return server.KillServer()
}
