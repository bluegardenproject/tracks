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
	"github.com/bluegardenproject/tracks/internal/v2/track"
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
				Tracks:  source.Daemon{Station: daemon.station},
				Watch:   daemon.watch,
				Open: func(number int) error {
					return trackwin.Switch(c, sessionName, strconv.Itoa(number), 0)
				},
				End: func(number int) error { return endTrack(cmd.Context(), daemon, c, number) },
				Resume: func(id string, recreate bool, progress func(string)) ([]string, error) {
					return resumeTrack(cmd.Context(), daemon, c, rpc.ResumeParams{ID: id, Recreate: recreate}, progress)
				},
				Promote: func(id string, progress func(string)) error {
					return promoteTrack(cmd.Context(), daemon, c, id, progress)
				},
				Archive: func(id string, force bool) ([]string, error) {
					return archiveTrack(cmd.Context(), daemon, rpc.ArchiveParams{ID: id, Force: force})
				},
				Lost: func(id string) ([]string, error) {
					return derailTrack(cmd.Context(), daemon, rpc.DerailParams{ID: id, Check: true})
				},
				Derail: func(id string, force bool) ([]string, error) {
					return derailTrack(cmd.Context(), daemon, rpc.DerailParams{ID: id, Force: force})
				},
				Unarchive: func(id string) error {
					return daemon.do(cmd.Context(), func(client rpc.Client) error { return client.Unarchive(cmd.Context(), id) })
				},
				SetFilter: func(f track.Filter) error {
					return daemon.do(cmd.Context(), func(client rpc.Client) error { return client.SetFilter(cmd.Context(), f) })
				},
				NewTrack: func() error {
					client, err := c.ClientOf(os.Getenv("TMUX_PANE"))
					if err != nil {
						return err
					}
					return openNewTrack(c, paths, client)
				},
				OpenURL:    openBrowser,
				Repos:      repoSource,
				ReposErr:   dbErr,
				Themes:     themes{c: c, paths: paths, version: version, command: command},
				ThemesDir:  paths.ThemesDir,
				Engines:    engines{path: paths.Settings},
				TrackTypes: trackTypes{path: paths.Settings},
				History:    history{path: paths.Settings},
				About:      about(paths),
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

// resumeTrack resumes the ended track through the daemon and switches
// to its window, or returns the worktrees the daemon couldn't find.
func resumeTrack(ctx context.Context, daemon daemonCalls, c *tmux.Client, p rpc.ResumeParams, progress func(string)) ([]string, error) {
	var r rpc.ResumeResult
	err := daemon.do(ctx, func(client rpc.Client) (err error) {
		r, err = client.Resume(ctx, p, progress)
		return err
	})
	if err != nil || len(r.Missing) > 0 {
		return r.Missing, err
	}
	return nil, switchToWindow(c, r.Window)
}

// promoteTrack promotes the track through the daemon and switches to
// its window.
func promoteTrack(ctx context.Context, daemon daemonCalls, c *tmux.Client, id string, progress func(string)) error {
	var r rpc.CreateResult
	err := daemon.do(ctx, func(client rpc.Client) (err error) {
		r, err = client.Promote(ctx, rpc.PromoteParams{ID: id}, progress)
		return err
	})
	if err != nil {
		return err
	}
	return switchToWindow(c, r.Window)
}

// switchToWindow switches the client to the track window with ID
// window, if it's still there.
func switchToWindow(c *tmux.Client, window string) error {
	infos, err := trackwin.List(c, sessionName)
	if err != nil {
		return err
	}
	for _, in := range infos {
		if in.Window == window {
			return trackwin.Switch(c, sessionName, strconv.Itoa(in.Number), 0)
		}
	}
	return nil
}

func archiveTrack(ctx context.Context, daemon daemonCalls, p rpc.ArchiveParams) (lost []string, err error) {
	err = daemon.do(ctx, func(client rpc.Client) (err error) {
		lost, err = client.Archive(ctx, p)
		return err
	})
	return lost, err
}

func derailTrack(ctx context.Context, daemon daemonCalls, p rpc.DerailParams) (lost []string, err error) {
	err = daemon.do(ctx, func(client rpc.Client) (err error) {
		lost, err = client.Derail(ctx, p)
		return err
	})
	return lost, err
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
			if !l.Open() {
				continue
			}
			for _, r := range l.Repos {
				if r.RepoID != 0 {
					uses = append(uses, repos.Use{RepoID: r.RepoID, Track: l.Name})
				}
			}
		}
		return uses, nil
	}
}
