package cli

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/spf13/cobra"
)

func newPathsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "print where Tracks v2 keeps its files",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := platform.Resolve()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "config dir   %s\ndata dir     %s\nworktrees    %s\ndaemon log   %s\ntmux socket  %s\n",
				p.ConfigDir, p.DataDir, p.Worktrees, p.Log, p.TmuxSocket)
			return err
		},
	}
}
