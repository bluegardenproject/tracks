package cli

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/spf13/cobra"
)

func newPathsCmd(profile profileFunc) *cobra.Command {
	return &cobra.Command{
		Use:   "paths",
		Short: "print where Tracks v2 keeps its files",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := platform.Resolve(profile())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "config dir   %s\ndata dir     %s\ntmux socket  %s\n",
				p.ConfigDir, p.DataDir, p.TmuxSocket)
			return err
		},
	}
}
