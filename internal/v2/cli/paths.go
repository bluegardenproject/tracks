package cli

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/spf13/cobra"
)

func newPathsCmd() *cobra.Command {
	var demo bool
	c := &cobra.Command{
		Use:   "paths",
		Short: "print where Tracks v2 keeps its files",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			profile := platform.Default
			if demo {
				profile = platform.Demo
			}
			p, err := platform.Resolve(profile)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "config dir   %s\ndata dir     %s\ntmux socket  %s\n",
				p.ConfigDir, p.DataDir, p.TmuxSocket)
			return err
		},
	}
	c.Flags().BoolVar(&demo, "demo", false, "show the playground's paths")
	return c
}
