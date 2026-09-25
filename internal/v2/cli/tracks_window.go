package cli

import (
	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/ui/style"
	"github.com/bluegardenproject/tracks/internal/v2/ui/tracksview"
	"github.com/spf13/cobra"
)

// newTracksWindowCmd runs the Tracks window in window 0.
func newTracksWindowCmd(profile profileFunc, version string) *cobra.Command {
	return &cobra.Command{
		Use:    "tracks-window",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			paths, err := platform.Resolve(profile())
			if err != nil {
				return err
			}
			command, err := selfCommand(profile())
			if err != nil {
				return err
			}
			t, _ := theme.Load(themePath(paths))
			apply := func(t theme.Theme, dark bool) error {
				return applyTheme(tmux.New(paths.TmuxSocket), paths, t, dark, version, command)
			}
			_, err = tea.NewProgram(tracksview.New(version, t, apply),
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
