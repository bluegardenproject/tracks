package tracks

import (
	"context"
	"errors"
	"fmt"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
)

// ReportExit records that track id's setup of repo subject (kind
// track.SetupError), or its dev server repo/server subject
// (track.ServerError), exited with code: anything but 0 is a failure,
// which Station shows until it's cleared. Nothing acts on it: the user
// decides what happens next.
func (s *Service) ReportExit(ctx context.Context, id, kind, subject string, code int) error {
	if kind != track.SetupError && kind != track.ServerError {
		return Problem(fmt.Sprintf("Tracks doesn't know the kind %q.", kind))
	}
	if subject == "" {
		return Problem("Name the repo, or repo/server, that exited.")
	}
	if code == 0 {
		return nil
	}
	err := s.Store.AddTrackError(ctx, id, track.Failure{Kind: kind, Subject: subject, Code: code})
	if isMissingTrack(err) {
		return Problem("That track is gone.")
	}
	return err
}

// DismissFailures clears track id's server and setup errors.
func (s *Service) DismissFailures(ctx context.Context, id string) error {
	_, err := s.Store.ClearTrackErrors(ctx, id, "", "")
	return err
}

// clearFailure clears a failure that starting or stopping the same
// setup or server again makes stale. It's best effort: the failure
// stays shown otherwise, until dismissed.
func (s *Service) clearFailure(ctx context.Context, id, kind, subject string) {
	_, _ = s.Store.ClearTrackErrors(ctx, id, kind, subject)
}

// isMissingTrack reports whether err is a write naming a track the
// database doesn't have.
func isMissingTrack(err error) bool {
	return errors.Is(err, store.ErrNotFound) || store.IsForeignKey(err)
}
