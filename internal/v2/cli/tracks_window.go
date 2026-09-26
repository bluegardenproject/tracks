package cli

import (
	"context"
	"errors"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/v2/demo"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/repos"
	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
	"github.com/bluegardenproject/tracks/internal/v2/ui/source"
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			paths, err := platform.Resolve(profile())
			if err != nil {
				return err
			}
			command, err := selfCommand(profile())
			if err != nil {
				return err
			}
			t, _ := theme.Load(themePath(paths))
			c := tmux.New(paths.TmuxSocket)
			var tracks source.Source = source.Windows{Tmux: c, Session: sessionName}
			openURL := openBrowser
			uses := trackUses(c)
			if profile() == platform.Demo {
				tracks = demo.Source{Windows: tracks}
				openURL = func(string) error { return errors.New("the playground's pull requests are made up") }
				uses = nil
			}
			var repoSource source.Repos
			db, dbErr := store.Open(cmd.Context(), paths.Database)
			if dbErr == nil {
				defer db.Close()
				repoSource = repos.Service{Store: db, Git: repos.Exec{}, Uses: uses}
			}
			window := tracksview.New(tracksview.Config{
				Version: version,
				Theme:   t,
				Apply: func(t theme.Theme, dark bool) error {
					return applyTheme(c, paths, t, dark, version, command)
				},
				Tracks: tracks,
				Open: func(number int) error {
					return trackwin.Switch(c, sessionName, strconv.Itoa(number), 0)
				},
				End: func(number int) error {
					return c.KillWindow("=" + sessionName + ":" + strconv.Itoa(number))
				},
				OpenURL:  openURL,
				Repos:    repoSource,
				ReposErr: dbErr,
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

// trackUses lists the repos of the session's track windows.
func trackUses(c *tmux.Client) repos.UsesFunc {
	return func(context.Context) ([]repos.Use, error) {
		infos, err := trackwin.List(c, sessionName)
		if err != nil {
			return nil, err
		}
		uses := make([]repos.Use, 0, len(infos))
		for _, in := range infos {
			if in.Repo != "" {
				uses = append(uses, repos.Use{Repo: in.Repo, Track: in.Name})
			}
		}
		return uses, nil
	}
}
