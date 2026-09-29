package tracks

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/workspace"
)

// Resume starts ended track id again: the worktrees Clean removed, a
// window under its name, and its engine on its session, saved last.
// progress is told each slow step. When a step fails, what Resume made
// is undone and the track stays ended.
func (s *Service) Resume(ctx context.Context, id string, progress func(string)) (Created, error) {
	t, release, err := s.hold(ctx, id)
	if err != nil {
		return Created{}, err
	}
	defer release()
	if t.Open() {
		return Created{}, Problem(t.Name + " is already open.")
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

	start, err := engine.Command(agents.Spec{
		Track: t, Program: info.Program, Auto: conf.AutoMode(), Resume: true,
		SocketDir: s.SocketDir, BinDir: s.BinDir,
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
	if err := s.Store.ReopenTrack(undo, t.ID, t.Name); err != nil {
		_ = s.Windows.Close(win.ID)
		removeWorktrees()
		return Created{}, fmt.Errorf("save the track: %w", err)
	}
	t.ClosedAt, t.CleanedAt = time.Time{}, time.Time{}
	return Created{Track: t, Window: win}, nil
}

// Clean removes ended track id's worktrees and keeps its branches.
// Unless force, it first looks for work that exists nowhere else and,
// when it finds some, returns it and removes nothing.
func (s *Service) Clean(ctx context.Context, id string, force bool) ([]workspace.Unsaved, error) {
	t, release, err := s.hold(ctx, id)
	if err != nil {
		return nil, err
	}
	defer release()
	switch {
	case !t.Kind.Worktrees():
		return nil, Problem(t.Name + " has no worktrees.")
	case t.Open():
		return nil, Problem("End " + t.Name + " before cleaning it.")
	case t.Cleaned():
		return nil, nil
	}
	if !force {
		unsaved, err := s.Worktrees.Unsaved(ctx, t)
		if err != nil || len(unsaved) > 0 {
			return unsaved, err
		}
	}
	if err := s.Worktrees.RemoveWorktrees(ctx, t.ID, t.Repos); err != nil {
		return nil, err
	}
	return nil, s.Store.CleanTrack(context.WithoutCancel(ctx), t.ID, s.now())
}
