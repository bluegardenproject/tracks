package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/bluegardenproject/tracks/internal/shellx"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/rpc"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
	"github.com/bluegardenproject/tracks/internal/v2/ui/addtrack"
)

// The New track form takes most of the window, and all of a small one.
const (
	formShare     = 80 // percent
	formMinWidth  = 72
	formMinHeight = 30
)

// openNewTrack opens the New track form over client, filled with draft
// unless it's "", and returns once it closes.
func openNewTrack(c *tmux.Client, paths platform.Paths, client, draft string) error {
	command, err := selfCommand()
	if err != nil {
		return err
	}
	width, height, err := c.ClientSize(client)
	if err != nil {
		return err
	}
	version, _ := tmux.InstalledVersion()
	t, _ := loadTheme(paths)
	return c.Popup(tmux.Popup{
		Client:     client,
		Width:      strconv.Itoa(min(width, max(formMinWidth, width*formShare/100))),
		Height:     strconv.Itoa(min(height, max(formMinHeight, height*formShare/100))),
		Command:    command + " popup add-track " + shellx.Quote(client) + draftFlag(draft),
		Background: t.Value(theme.OverlayBg),
	}, version)
}

// formHere opens the New track form over the client showing this pane,
// filled with draft unless it's "".
func formHere(c *tmux.Client, paths platform.Paths, draft string) error {
	client, err := c.ClientOf(os.Getenv("TMUX_PANE"))
	if err != nil {
		return err
	}
	return openNewTrack(c, paths, client, draft)
}

func draftFlag(draft string) string {
	if draft == "" {
		return ""
	}
	return " --draft " + shellx.Quote(draft)
}

// addTrack runs the New track form over client, filled with draft
// unless it's "", and shows client the track it creates.
func addTrack(ctx context.Context, version, client, draft string) error {
	paths, err := platform.Resolve()
	if err != nil {
		return err
	}
	t, _ := loadTheme(paths)
	c := tmux.New(paths.TmuxSocket)
	names, reposErr := repoNames(ctx, paths)
	conf := addtrack.Config{Theme: t, Repos: names, ReposErr: reposErr, Create: creator(c, paths, version, client)}
	if s, err := settings.Load(paths.Settings); err == nil {
		conf.RunsOn, conf.Engines, conf.Models = runsOn(s), addedEngines(s), listModels
	}
	form := addtrack.New(conf)
	if draft != "" {
		if req, err := (rpc.Client{Socket: paths.Socket}).Draft(ctx, draft); err != nil {
			form = form.Tell("Couldn't read the draft: " + err.Error())
		} else {
			form = form.Fill(req)
		}
	}
	done, err := runPopup(ctx, form)
	if err != nil {
		return err
	}
	if m, ok := done.(addtrack.Model); ok && m.Made() != nil {
		return c.SwitchClient(client, m.Made().Window)
	}
	return nil
}

// runsOn is what each type of track runs on, as Create would pick it:
// the engine, and the type's own model on top of the engine's default.
func runsOn(s settings.Settings) map[track.Kind]addtrack.RunsOn {
	on := map[track.Kind]addtrack.RunsOn{}
	for _, k := range track.Kinds {
		id, _ := s.RunsOn(string(k))
		if _, ok := agents.ByID(id); !ok {
			continue
		}
		var model string
		if d := s.Tracks.Get(string(k)); d != nil {
			model = d.Model
		}
		on[k] = addtrack.RunsOn{Engine: id, Model: model}
	}
	return on
}

// addedEngines are the engines added on the Engines tab.
func addedEngines(s settings.Settings) []addtrack.Engine {
	var out []addtrack.Engine
	for _, e := range agents.All {
		if conf := s.Engines.Get(e.ID); conf != nil {
			out = append(out, addtrack.Engine{
				ID: e.ID, Name: e.Name, Default: conf.Model,
				Models: e.Models, Added: conf.Models, Lists: e.ListsModels,
			})
		}
	}
	return out
}

// listModels asks the CLI of the engine with id for its models.
func listModels(ctx context.Context, id string) ([]agents.Model, error) {
	e, ok := agents.ByID(id)
	if !ok {
		return nil, fmt.Errorf("tracks doesn't know the engine %q", id)
	}
	found, err := agents.Check(ctx, e)
	if err != nil {
		return nil, err
	}
	return agents.ListModels(ctx, found.Path)
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
