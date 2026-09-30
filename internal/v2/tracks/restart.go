package tracks

import (
	"context"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Restart starts open track id's agent again in its pane, once it
// exited: on its session, or on the original prompt when the session
// never started, as when the agent's binary wasn't found.
func (s *Service) Restart(ctx context.Context, id string, progress func(string)) (Created, error) {
	t, release, err := s.hold(ctx, id)
	if err != nil {
		return Created{}, err
	}
	defer release()
	if !t.Open() {
		return Created{}, Problem(t.Name + " has ended: resume it instead.")
	}
	infos, err := s.Windows.List()
	if err != nil {
		return Created{}, err
	}
	window, exit := "", ""
	for _, in := range infos {
		if in.Track == t.ID {
			window, exit = in.Window, in.Exit
		}
	}
	switch {
	case window == "":
		return Created{}, Problem(t.Name + " has no window.")
	case t.Exit == "" && exit == "":
		return Created{}, Problem(t.Name + "'s agent is still running.")
	}
	info, known := agents.ByID(t.Engine)
	engine, ok := s.engine(t.Engine)
	if !known || !ok {
		return Created{}, Problem(fmt.Sprintf("Tracks doesn't know the engine %s.", t.Engine))
	}
	set, err := s.Settings()
	if err != nil {
		return Created{}, err
	}
	conf := set.Engines.Get(t.Engine)
	if conf == nil {
		return Created{}, Problem(fmt.Sprintf("Add %s on the Engines tab to restart this track.", info.Name))
	}

	resume := engine.Started(t.Session)
	var drafts []string
	if !resume {
		if drafts, err = s.draftRepos(ctx, t.Repos); err != nil {
			return Created{}, err
		}
	}
	hooks, err := s.installHooks(t)
	if err != nil {
		return Created{}, err
	}
	start, err := engine.Command(agents.Spec{
		Track: t, Program: info.Program, Auto: conf.AutoMode(), Resume: resume, DraftPRs: drafts,
		SocketDir: s.SocketDir, BinDir: s.BinDir, Hooks: hooks,
	})
	if err != nil {
		return Created{}, err
	}
	progress(fmt.Sprintf("Starting %s…", info.Name))
	win, err := s.Windows.Respawn(window, windowSpec(t, info, start))
	if err != nil {
		return Created{}, fmt.Errorf("restart %s: %w", info.Name, err)
	}
	if err := s.Report(context.WithoutCancel(ctx), t.ID, track.Resumed); err != nil {
		return Created{}, err
	}
	t.State = track.State{}
	return Created{Track: t, Window: win}, nil
}
