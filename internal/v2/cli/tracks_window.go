package cli

import (
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/tracksview"
	"github.com/spf13/cobra"
)

// newTracksWindowCmd runs the Tracks window in window 0.
func newTracksWindowCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:    "tracks-window",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			_, err := tea.NewProgram(tracksview.New(version),
				tea.WithContext(c.Context()),
				tea.WithColorProfile(style.Profile()),
			).Run()
			if c.Context().Err() != nil {
				return nil
			}
			return err
		},
	}
}
