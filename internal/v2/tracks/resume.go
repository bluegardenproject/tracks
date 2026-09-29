package tracks

import (
	"context"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// Missing is Resume's answer when some of the track's worktrees are
// gone, as Archive leaves them. Resume re-creates them when asked to,
// and their branches if those are gone too.
type Missing []track.Repo

func (m Missing) Error() string {
	names := make([]string, len(m))
	for i, r := range m {
		names[i] = r.Name
	}
	return "the worktree couldn't be found for " + strings.Join(names, ", ")
}

// Resume starts ended track id again: a window under its name and its
// engine on its session, saved last. A worktree that's gone is Missing,
// unless recreate, which re-creates it first. progress is told each
// slow step. When a step fails, what Resume made is undone and the
// track stays ended.
func (s *Service) Resume(ctx context.Context, id string, recreate bool, progress func(string)) (Created, error) {
	t, release, err := s.hold(ctx, id)
	if err != nil {
		return Created{}, err
	}
	defer release()
	switch {
	case t.Open():
		return Created{}, Problem(t.Name + " is already open.")
	}
	if missing := s.Worktrees.Missing(t); len(missing) > 0 && !recreate {
		return Created{}, Missing(missing)
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
		return Created{}, Problem(fmt.Sprintf("Add %s on the Engines tab to resume this track.", info.Name))
	}

	name, releaseName, err := s.claimName(t)
	if err != nil {
		return Created{}, err
	}
	defer releaseName()
	t.Name = name

	made, err := s.Worktrees.Restore(ctx, t, progress)
	if err != nil {
		return Created{}, err
	}
	undo := context.WithoutCancel(ctx)
	removeWorktrees := func() { _ = s.Worktrees.RemoveWorktrees(undo, t.ID, made) }

	hooks, err := s.installHooks(t)
	if err != nil {
		removeWorktrees()
		return Created{}, err
	}
	start, err := engine.Command(agents.Spec{
		Track: t, Program: info.Program, Auto: conf.AutoMode(), Resume: true,
		SocketDir: s.SocketDir, BinDir: s.BinDir, Hooks: hooks,
	})
	if err != nil {
		removeWorktrees()
		return Created{}, err
	}

	progress(fmt.Sprintf("Starting %s…", info.Name))
	win, err := s.open(t, info, start)
	if err != nil {
		removeWorktrees()
		return Created{}, err
	}
	if err := s.reopen(undo, t.ID, t.Name); err != nil {
		_ = s.Windows.Close(win.ID)
		removeWorktrees()
		return Created{}, fmt.Errorf("save the track: %w", err)
	}
	t.State = track.State{}
	return Created{Track: t, Window: win}, nil
}
