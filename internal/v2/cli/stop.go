package cli

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "stop Tracks v2 and its tmux server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			paths, err := platform.Resolve(platform.Default)
			if err != nil {
				return err
			}
			server := tmux.New(paths.TmuxSocket)
			if !server.HasSession(sessionName) {
				_, err := fmt.Fprintln(c.OutOrStdout(), "Tracks v2 isn't running.")
				return err
			}
			if err := server.KillServer(); err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), "Stopped Tracks v2.")
			return err
		},
	}
}
