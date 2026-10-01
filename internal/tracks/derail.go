package tracks

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/workspace"
)

// Derail deletes ended track id for good: its worktrees, a work track's
// local branches, its hooks and its record. Pushed branches and PRs
// stay on GitHub. Unless force, it first looks for work that would be
// lost and, when it finds some, returns it and deletes nothing.
func (s *Service) Derail(ctx context.Context, id string, force bool) ([]workspace.Unsaved, error) {
	t, release, err := s.holdEnded(ctx, id)
	if err != nil {
		return nil, err
	}
	defer release()
	if t.Kind.Worktrees() {
		if lost, err := s.lost(ctx, t, force); err != nil || len(lost) > 0 {
			return lost, err
		}
		if err := s.Worktrees.Discard(ctx, t); err != nil {
			return nil, err
		}
	}
	s.removeHooks(t.ID)
	return nil, s.Store.DeleteTrack(context.WithoutCancel(ctx), t.ID)
}

// Lost is Derail's check alone: the work derailing ended track id would
// lose. It deletes nothing.
func (s *Service) Lost(ctx context.Context, id string) ([]workspace.Unsaved, error) {
	t, release, err := s.holdEnded(ctx, id)
	if err != nil {
		return nil, err
	}
	defer release()
	if !t.Kind.Worktrees() {
		return nil, nil
	}
	return s.Worktrees.Lost(ctx, t)
}

// holdEnded holds track id for Derail, which takes only ended tracks.
func (s *Service) holdEnded(ctx context.Context, id string) (track.Track, func(), error) {
	t, release, err := s.hold(ctx, id)
	if err != nil {
		return t, nil, err
	}
	if t.Open() {
		release()
		return t, nil, Problem("End " + t.Name + " before derailing it.")
	}
	return t, release, nil
}
