package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newTracksWindowCmd runs in window 0 until the real Tracks window
// replaces it.
func newTracksWindowCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "tracks-window",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			fmt.Fprint(c.OutOrStdout(), "Tracks v2: placeholder for the Tracks window.\n\n"+
				"Detach: Ctrl+b d    Stop: ./tracks --new-app stop\n")
			<-c.Context().Done()
			return nil
		},
	}
}
