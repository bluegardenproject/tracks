package tracks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/train"
)

// SyncTrains keeps each open track's Platform in step with its trains:
// open while it has one, closed when it has none. A train's status
// comes from tmux and its run, not the store, so SyncTrains also tells
// Changes when a track's trains changed since it last looked, for the
// Platforms to read them again. It reads the windows, their Platforms
// and the reviewers once each, whatever the number of tracks.
func (s *Service) SyncTrains(ctx context.Context) error {
	infos, err := s.Windows.List()
	if err != nil {
		return err
	}
	platforms, err := s.Windows.Platforms()
	if err != nil {
		return err
	}
	reviewers := map[string]ReviewerState{}
	if s.Reviewers != nil && s.HooksDir != "" {
		if reviewers, err = s.Reviewers.Reviewers(); err != nil {
			return err
		}
	}
	seen := map[string]string{}
	var errs []error
	for _, in := range infos {
		// A track whose agent exited is still synced: its window and its
		// trains run on, and the Platform types nothing into the agent.
		if in.Track == "" || s.isBusy(in.Track) {
			continue
		}
		r := reviewers[in.Track]
		s.reviewMu.Lock()
		run := s.reviewerRun(in.Track, r)
		s.reviewMu.Unlock()
		trains := s.reviewerTrains(in.Track, r, run)
		if len(trains) > 0 {
			seen[in.Track] = trainsKey(trains)
		}
		pane := platforms[in.Window]
		switch {
		case len(trains) > 0 && pane == "":
			errs = append(errs, s.Windows.OpenPlatform(in.Window, s.platformCommand(in.Track)))
		case len(trains) == 0 && pane != "":
			errs = append(errs, s.Windows.ClosePlatform(in.Window))
		}
	}
	s.mu.Lock()
	changed := len(s.trainsSeen) != len(seen)
	for id, key := range seen {
		if s.trainsSeen[id] != key {
			changed = true
		}
	}
	s.trainsSeen = seen
	s.mu.Unlock()
	if changed && s.Changes != nil {
		s.Changes.Notify()
	}
	return errors.Join(errs...)
}

// trainsKey is what of trains a Platform shows, to tell a change.
func trainsKey(trains []train.Train) string {
	var b strings.Builder
	for _, t := range trains {
		fmt.Fprintf(&b, "%s %s %s %s|", t.Handle, t.Engine, t.Status.ID, t.Outcome.ID)
	}
	return b.String()
}

// platformCommand runs track id's Platform: tracks platform from
// Tracks' own folder, with the track's ID.
func (s *Service) platformCommand(id string) string {
	env := "TRACKS_ID=" + shellx.Quote(id) + " TRACKS_SOCKET_DIR=" + shellx.Quote(s.SocketDir)
	tracks := "tracks"
	if s.BinDir != "" {
		tracks = shellx.Quote(filepath.Join(s.BinDir, "tracks"))
	}
	return env + " " + tracks + " platform"
}
