package cli

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/rpc"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
	"github.com/bluegardenproject/tracks/internal/v2/ui/addtrack"
)

// addTrack runs the New track form over client, and shows client the
// track it creates.
func addTrack(ctx context.Context, version, client string) error {
	paths, err := platform.Resolve()
	if err != nil {
		return err
	}
	t, _ := loadTheme(paths)
	c := tmux.New(paths.TmuxSocket)
	names, reposErr := repoNames(ctx, paths)
	conf := addtrack.Config{Theme: t, Repos: names, ReposErr: reposErr, Create: creator(c, paths, version, client)}
	if s, err := settings.Load(paths.Settings); err == nil {
		id := s.Engines.DefaultID()
		if e, ok := agents.ByID(id); ok {
			conf.Engine, conf.Model = e.Name, s.Engines.Get(id).Model
		}
	}
	done, err := runPopup(ctx, addtrack.New(conf))
	if err != nil {
		return err
	}
	if m, ok := done.(addtrack.Model); ok && m.Made() != nil {
		return c.SwitchClient(client, m.Made().Window)
	}
	return nil
}

// creator creates tracks through a daemon of this build, which tells
// client how it went if the form is closed before.
func creator(c *tmux.Client, paths platform.Paths, version, client string) addtrack.CreateFunc {
	return func(ctx context.Context, req tracks.Request, progress func(string)) (addtrack.Created, error) {
		d, err := ensureDaemon(ctx, c, paths, version)
		if err != nil {
			return addtrack.Created{}, err
		}
		r, err := d.Create(ctx, rpc.CreateParams{Request: req, Client: client}, progress)
		return addtrack.Created{Name: r.Name, Window: r.Window}, err
	}
}
