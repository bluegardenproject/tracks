package tracks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/workspace"
)

// ArchiveAfter is how long after it ended auto-archive takes a track.
const ArchiveAfter = 7 * 24 * time.Hour

// Archive takes ended track id out of Station, removing its worktrees
// first as Clean does. Unless force, it returns the unsaved work it
// finds instead, and changes nothing.
func (s *Service) Archive(ctx context.Context, id string, force bool) ([]workspace.Unsaved, error) {
	return s.archive(ctx, id, force, false)
}

// Unarchive puts archived track id back in Station.
func (s *Service) Unarchive(ctx context.Context, id string) error {
	_, release, err := s.hold(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	return s.Report(ctx, id, track.Unarchived)
}

// archive is Archive; keep archives a track with unsaved work and
// leaves its worktrees.
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
	if t.Kind.Worktrees() && !t.Cleaned() {
		unsaved, err := s.clean(ctx, t, force)
		if err != nil || (len(unsaved) > 0 && !keep) {
			return unsaved, err
		}
	}
	return nil, s.Report(context.WithoutCancel(ctx), id, track.Archived)
}

// AutoArchive archives the tracks that ended more than ArchiveAfter ago,
// when the setting is on, and returns their names. A track with a PR
// still open stays; one with unsaved work stays too, unless the setting
// says to keep its worktrees.
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
