package cli

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/rpc"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
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
			if err := stopDaemon(c.Context(), rpc.Client{Socket: paths.Socket}); err != nil {
				return err
			}
			if err := server.KillServer(); err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "%s stopped.\n", name)
			return err
		},
	}
}
