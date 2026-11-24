package cli

import (
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/train"
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
			id := os.Getenv("TRACKS_ID")
			if id == "" {
				return errNoTrack("platform")
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			t, _ := loadTheme(paths)
			client := rpc.Client{Socket: paths.Socket}
			daemon := daemonCalls{tmux: tmux.New(paths.TmuxSocket), paths: paths, version: version}
			m := uiplatform.New(uiplatform.Config{
				Theme:     t,
				Trains:    func() ([]uiplatform.Entry, error) { return platformEntries(client.TrainList(cmd.Context(), id)) },
				Watch:     daemon.watch,
				LoadTheme: func() (theme.Theme, error) { return loadTheme(paths) },
				Collect:   func() error { return client.RelayCollect(cmd.Context(), id) },
			})
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context()), tea.WithColorProfile(style.Profile())).Run()
			if cmd.Context().Err() != nil {
				return nil
			}
			return err
		},
	}
}

// platformEntries are trains, from the daemon, as the Platform shows
// them.
func platformEntries(trains []rpc.Train, err error) ([]uiplatform.Entry, error) {
	if err != nil {
		return nil, err
	}
	out := make([]uiplatform.Entry, len(trains))
	for i, t := range trains {
		out[i] = uiplatform.Entry{Handle: t.Handle, Status: train.StatusOf(t.StatusID), Outcome: train.StatusOf(t.OutcomeID), Held: t.Held}
	}
	return out, nil
}
