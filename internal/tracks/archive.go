package tracks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/workspace"
)

// ArchiveAfter is how long after it ended auto-archive takes a track.
const ArchiveAfter = 7 * 24 * time.Hour

// Archive takes ended track id out of Station, closing it: it removes
// its worktrees and a work track's local branches first, as Derail
// does. Unless force, it returns the work that would be lost instead,
// and changes nothing.
func (s *Service) Archive(ctx context.Context, id string, force bool) ([]workspace.Unsaved, error) {
	return s.archive(ctx, id, force, false)
}

// Unarchive puts archived track id back in Station, done.
func (s *Service) Unarchive(ctx context.Context, id string) error {
	_, release, err := s.hold(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	return s.Report(ctx, id, track.Unarchived)
}

// archive is Archive; keep archives a track with work that would be
// lost and leaves its worktrees and branches.
func (s *Service) archive(ctx context.Context, id string, force, keep bool) ([]workspace.Unsaved, error) {
	t, release, err := s.hold(ctx, id)
	if err != nil {
		return nil, err
	}
	defer release()
	switch {
	case t.Open():
		return nil, Problem("End " + t.Name + " before archiving it.")
	case t.Archived():
		return nil, nil
	}
	if t.Kind.Worktrees() {
		lost, err := s.lost(ctx, t, force)
		switch {
		case err != nil:
			return nil, err
		case len(lost) > 0 && !keep:
			return lost, nil
		case len(lost) == 0:
			if err := s.discard(ctx, t); err != nil {
				return nil, err
			}
		}
	}
	return nil, s.Report(context.WithoutCancel(ctx), id, track.Archived)
}

// lost is what discarding t would lose; nothing when force.
func (s *Service) lost(ctx context.Context, t track.Track, force bool) ([]workspace.Unsaved, error) {
	if force {
		return nil, nil
	}
	return s.Worktrees.Lost(ctx, t)
}

// discard removes held t's worktrees, a work track's local branches and
// its hooks. The record keeps the branches under the names the agent
// gave them, which Resume re-creates them under.
func (s *Service) discard(ctx context.Context, t track.Track) error {
	for i, r := range s.Worktrees.Branches(ctx, t) {
		if r.Branch != t.Repos[i].Branch {
			if err := s.Store.SetBranch(ctx, t.ID, i, r.Branch); err != nil {
				return err
			}
		}
	}
	if err := s.Worktrees.Discard(ctx, t); err != nil {
		return err
	}
	s.removeHooks(t.ID)
	return s.Report(context.WithoutCancel(ctx), t.ID, track.Cleaned)
}

// AutoArchive archives the tracks that ended more than ArchiveAfter ago,
// when the setting is on, and returns their names. A track with a PR
// still open stays; one with work that would be lost stays too, unless
// the setting says to archive it with its worktrees and branches.
func (s *Service) AutoArchive(ctx context.Context) ([]string, error) {
	st, err := s.Settings()
	if err != nil || !st.History.AutoArchive {
		return nil, err
	}
	old, err := s.Store.EndedBefore(ctx, s.now().Add(-ArchiveAfter))
	if err != nil {
		return nil, err
	}
	var archived []string
	var errs []error
	for _, t := range old {
		if track.PRStatus(t.PRs).ID == track.PRsOpen.ID || ctx.Err() != nil {
			continue
		}
		unsaved, err := s.archive(ctx, t.ID, false, st.History.KeepUnsaved())
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", t.Name, err))
		case len(unsaved) == 0 || st.History.KeepUnsaved():
			archived = append(archived, t.Name)
		}
	}
	return archived, errors.Join(errs...)
}
