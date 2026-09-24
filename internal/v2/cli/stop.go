package cli

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/spf13/cobra"
)

func newStopCmd(profile profileFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "stop Tracks v2 and its tmux server",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			name := "Tracks v2"
			if profile() == platform.Demo {
				name = "The Tracks v2 playground"
			}
			paths, err := platform.Resolve(profile())
			if err != nil {
				return err
			}
			server := tmux.New(paths.TmuxSocket)
			if !server.HasSession(sessionName) {
				_, err := fmt.Fprintf(c.OutOrStdout(), "%s isn't running.\n", name)
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
