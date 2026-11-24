package tracks

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/train"
)

// SyncTrains keeps each open track's Platform in step with its trains
// and its PRs' comments: open while it has one or the other, closed when
// it has neither. A train's status comes from tmux and its run, not the
// store, so SyncTrains also tells Changes when a track's trains changed
// since it last looked, for the Platforms to read them again, brings the
// trains' overlay headers up to their status, and tells the user when
// one waits on them, as on Claude's trust question, which only its
// overlay shows. It reads the windows, their Platforms and the reviewers
// once each, whatever the number of tracks.
func (s *Service) SyncTrains(ctx context.Context) error {
	infos, err := s.Windows.List()
	if err != nil {
		return err
	}
	platforms, err := s.Windows.Platforms()
	if err != nil {
		return err
	}
	// The reviewers' panes and runs are read together, as StartReview
	// and CloseReviewer change them together.
	reviewers := map[string]ReviewerState{}
	runs := map[string]reviewerRunState{}
	if s.Reviewers != nil && s.HooksDir != "" {
		// A review starting holds the lock across a network call; the
		// tick doesn't wait for it, the next one catches up.
		if !s.reviewMu.TryLock() {
			return nil
		}
		reviewers, err = s.Reviewers.Reviewers()
		for id, r := range reviewers {
			runs[id] = s.reviewerRun(id, r)
		}
		s.reviewMu.Unlock()
		if err != nil {
			return err
		}
	}
	s.mu.Lock()
	last := s.trainsSeen
	s.mu.Unlock()
	open, err := s.Store.OpenTracks(ctx)
	if err != nil {
		return err
	}
	tracks := map[string]track.Track{}
	for _, t := range open {
		tracks[t.ID] = t
	}
	seen := map[string]string{}
	live, windowed := map[string]bool{}, map[string]bool{}
	var errs []error
	for _, in := range infos {
		if in.Track == "" {
			continue
		}
		windowed[in.Track] = true
		// A busy track, being restarted or promoted, keeps what was seen
		// of it: its Platform is left as it is meanwhile.
		if s.isBusy(in.Track) {
			if key, ok := last[in.Track]; ok {
				seen[in.Track] = key
			}
			continue
		}
		// A track whose agent exited is still synced: its window and its
		// trains run on, and the Platform types nothing into the agent.
		trains := s.reviewerTrains(in.Track, reviewers[in.Track], runs[in.Track])
		s.mu.Lock()
		prs := prSignals(tracks[in.Track], s.comments[in.Track])
		s.mu.Unlock()
		// The Platform shows while it has something: a train, or a PR's
		// comments under Signals.
		shown := len(trains) > 0 || len(prs) > 0
		if shown {
			seen[in.Track] = trainsKey(trains) + fmt.Sprint(prs)
		}
		for _, tr := range trains {
			session := reviewers[in.Track].Session
			live[session] = true
			s.showBadge(session, actions.Badges(tr.Status, tr.Outcome))
			if tr.Status == track.ActionRequired {
				s.tellWaiting(ctx, in.Track, tr, runs[in.Track].run)
			}
		}
		pane := platforms[in.Window]
		switch {
		case shown && pane == "":
			errs = append(errs, s.Windows.OpenPlatform(in.Window, s.platformCommand(in.Track)))
		case !shown && pane != "":
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
	for session := range s.badges {
		if !live[session] {
			delete(s.badges, session)
		}
	}
	for id := range s.trainTold {
		if !windowed[id] {
			delete(s.trainTold, id)
		}
	}
	s.mu.Unlock()
	if changed && s.Changes != nil {
		s.Changes.Notify()
	}
	return errors.Join(errs...)
}

// Signals are what a track's Platform shows under Signals: its PRs with
// open comments, and how many relay messages the user hasn't seen.
type Signals struct {
	PRs      []PRSignal
	RelayNew int
}

// Signals are track id's Signals.
func (s *Service) Signals(ctx context.Context, id string) (Signals, error) {
	t, err := s.relayTrack(ctx, id)
	if err != nil {
		return Signals{}, err
	}
	n, err := s.Store.RelayUnseen(ctx, id, relay.UserHandle)
	if err != nil {
		return Signals{}, err
	}
	s.mu.Lock()
	prs := prSignals(t, s.comments[id])
	s.mu.Unlock()
	return Signals{PRs: prs, RelayNew: n}, nil
}

// trainsKey is what of trains a Platform shows, to tell a change.
func trainsKey(trains []train.Train) string {
	var b strings.Builder
	for _, t := range trains {
		fmt.Fprintf(&b, "%s %s %s %s %v|", t.Handle, t.Engine, t.Status.ID, t.Outcome.ID, t.Held)
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

func (w TmuxWindows) Platforms() (map[string]string, error) {
	panes, err := w.Tmux.ListSessionPanes(w.Session)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, p := range panes {
		if p.Role == trackwin.RolePlatform {
			out[p.Window] = p.ID
		}
	}
	return out, nil
}

// OpenPlatform kills a pane it opened but couldn't label: unlabelled,
// it wouldn't be found as the Platform, and the next tick would open
// another.
func (w TmuxWindows) OpenPlatform(window, command string) error {
	pane, err := trackwin.OpenPlatform(w.Tmux, window, "", trackwin.Process{Title: "trains", Command: command})
	if err != nil && pane != "" {
		_ = w.Tmux.KillPane(pane)
	}
	return err
}

func (w TmuxWindows) ClosePlatform(window string) error {
	pane, err := trackwin.Platform(w.Tmux, window)
	if err != nil || pane == "" {
		return err
	}
	return w.Tmux.KillPane(pane)
}
