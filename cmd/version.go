package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	register(&cobra.Command{
		Use:   "version",
		Short: "print binary version and build time",
		Args:  cobra.NoArgs,
		Run: func(c *cobra.Command, args []string) {
			fmt.Printf("tracks %s (built %s)\n", Version, BuildTime)
		},
	})
}
