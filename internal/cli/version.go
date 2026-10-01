package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newVersionCmd prints the version and build time, as v1's does; the
// install script runs it to check the binary it installed.
func newVersionCmd(version, built string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version and build time",
		Args:  cobra.NoArgs,
		Run: func(c *cobra.Command, _ []string) {
			fmt.Fprintf(c.OutOrStdout(), "tracks %s (built %s)\n", version, built)
		},
	}
}
