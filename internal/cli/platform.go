package cli

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tmux"
	uiplatform "github.com/bluegardenproject/tracks/internal/ui/platform"
	"github.com/bluegardenproject/tracks/internal/ui/style"
	"github.com/spf13/cobra"
)

// newPlatformCmd runs a track's Platform, the pane on its window's left
// that lists its trains. The daemon opens and closes it.
func newPlatformCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:    "platform",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, id, err := trackClient("platform")
			if err != nil {
				return err
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			t, _ := loadTheme(paths)
			daemon := daemonCalls{tmux: tmux.New(paths.TmuxSocket), paths: paths, version: version}
			m := uiplatform.New(uiplatform.Config{
				Theme:  t,
				Trains: func() ([]rpc.Train, error) { return client.TrainList(cmd.Context(), id) },
				Watch: func(ctx context.Context, changed func()) error {
					return daemon.watch(ctx, changed)
				},
			})
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context()), tea.WithColorProfile(style.Profile())).Run()
			if cmd.Context().Err() != nil {
				return nil
			}
			return err
		},
	}
}
