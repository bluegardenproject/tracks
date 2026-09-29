package tracks

import (
	"context"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/workspace"
)

// Missing is Resume's answer when some of the track's worktrees are
// gone, which only Clean should do. Resume re-creates them when asked
// to.
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
	case t.Cleaned():
		return Created{}, Problem(t.Name + " was cleaned, so it can't be resumed.")
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

// Clean removes ended track id's worktrees and keeps its branches.
// Unless force, it first looks for work that exists nowhere else and,
// when it finds some, returns it and removes nothing.
func (s *Service) Clean(ctx context.Context, id string, force bool) ([]workspace.Unsaved, error) {
	t, release, err := s.hold(ctx, id)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := cleanable(t); err != nil || t.Cleaned() {
		return nil, err
	}
	if !force {
		unsaved, err := s.Worktrees.Unsaved(ctx, t)
		if err != nil || len(unsaved) > 0 {
			return unsaved, err
		}
	}
	// The branches stay under the names the agent gave them.
	for i, r := range s.Worktrees.Branches(ctx, t) {
		if r.Branch != t.Repos[i].Branch {
			if err := s.Store.SetBranch(ctx, t.ID, i, r.Branch); err != nil {
				return nil, err
			}
		}
	}
	if err := s.Worktrees.RemoveWorktrees(ctx, t.ID, t.Repos); err != nil {
		return nil, err
	}
	s.removeHooks(t.ID)
	return nil, s.Report(context.WithoutCancel(ctx), t.ID, track.Cleaned)
}

// Unsaved is Clean's check alone: the work in ended track id's
// worktrees that exists nowhere else. It removes nothing.
func (s *Service) Unsaved(ctx context.Context, id string) ([]workspace.Unsaved, error) {
	t, release, err := s.hold(ctx, id)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := cleanable(t); err != nil || t.Cleaned() {
		return nil, err
	}
	return s.Worktrees.Unsaved(ctx, t)
}

func cleanable(t track.Track) error {
	switch {
	case !t.Kind.Worktrees():
		return Problem(t.Name + " has no worktrees.")
	case t.Open():
		return Problem("End " + t.Name + " before cleaning it.")
	}
	return nil
}
