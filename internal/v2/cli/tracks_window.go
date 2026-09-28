package cli

import (
	"context"
	"os"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/repos"
	"github.com/bluegardenproject/tracks/internal/v2/rpc"
	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			command, err := selfCommand()
			if err != nil {
				return err
			}
			t, _ := loadTheme(paths)
			c := tmux.New(paths.TmuxSocket)
			daemon := daemonCalls{tmux: c, paths: paths, version: version}
			var repoSource source.Repos
			db, dbErr := store.Open(cmd.Context(), paths.Database)
			if dbErr == nil {
				defer db.Close()
				repoSource = repos.Service{Store: db, Git: repos.Exec{}, Uses: trackUses(daemon)}
			}
			window := tracksview.New(tracksview.Config{
				Version: version,
				Theme:   t,
				Tracks:  source.Daemon{List: daemon.list},
				Open: func(number int) error {
					return trackwin.Switch(c, sessionName, strconv.Itoa(number), 0)
				},
				End: func(number int) error { return endTrack(cmd.Context(), daemon, c, number) },
				NewTrack: func() error {
					client, err := c.ClientOf(os.Getenv("TMUX_PANE"))
					if err != nil {
						return err
					}
					return openNewTrack(c, paths, client)
				},
				OpenURL:   openBrowser,
				Repos:     repoSource,
				ReposErr:  dbErr,
				Themes:    themes{c: c, paths: paths, version: version, command: command},
				ThemesDir: paths.ThemesDir,
				Engines:   engines{path: paths.Settings},
				About:     about(paths),
			})
			_, err = tea.NewProgram(window,
				tea.WithContext(cmd.Context()),
				tea.WithColorProfile(style.Profile()),
			).Run()
			if cmd.Context().Err() != nil {
				return nil
			}
			return err
		},
	}
}

// about is what Settings shows under About.
func about(paths platform.Paths) [][2]string {
	return [][2]string{
		{"Config folder", paths.ConfigDir},
		{"Settings file", paths.Settings},
		{"Themes folder", paths.ThemesDir},
		{"Data folder", paths.DataDir},
		{"Database", paths.Database},
		{"Worktrees folder", paths.Worktrees},
		{"Daemon log", paths.Log},
		{"tmux socket", paths.TmuxSocket},
	}
}

// endTrack ends the track in window number through the daemon, which
// records it. A window without a track is only closed.
func endTrack(ctx context.Context, daemon daemonCalls, c *tmux.Client, number int) error {
	listed, err := daemon.list(ctx)
	if err != nil {
		return err
	}
	for _, l := range listed {
		if l.Number == number {
			return daemon.do(ctx, func(client rpc.Client) error { return client.End(ctx, l.ID) })
		}
	}
	return c.KillWindow("=" + sessionName + ":" + strconv.Itoa(number))
}

// trackUses lists the repos of the open tracks.
func trackUses(daemon daemonCalls) repos.UsesFunc {
	return func(ctx context.Context) ([]repos.Use, error) {
		listed, err := daemon.list(ctx)
		if err != nil {
			return nil, err
		}
		var uses []repos.Use
		for _, l := range listed {
			for _, r := range l.Repos {
				if r.RepoID != 0 {
					uses = append(uses, repos.Use{RepoID: r.RepoID, Track: l.Name})
				}
			}
		}
		return uses, nil
	}
}
