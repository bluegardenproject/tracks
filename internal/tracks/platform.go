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

// SyncTrains keeps a Platform open on each open track's window,
// whatever it shows, windows opened before this daemon included. A
// train's status comes from tmux and its run, not the store, so
// SyncTrains also tells Changes when a track's trains changed
// since it last looked, for the Platforms to read them again, brings the
// trains' overlay headers up to their status, and tells the user when
// one waits on them, as on Claude's trust question, which only its
// overlay shows; it puts each Platform back to its width. It reads the
// windows, their Platforms and the reviewers a fixed number of times,
// whatever the number of tracks.
func (s *Service) SyncTrains(ctx context.Context) error {
	infos, err := s.Windows.List()
	if err != nil {
		return err
	}
	platforms, err := s.Windows.Platforms()
	if err != nil {
		return err
	}
	// A Platform that couldn't be fitted, its window closing meanwhile,
	// doesn't hold the rest back.
	fitErr := s.Windows.FitPlatforms()
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
	errs := []error{fitErr}
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
		if len(trains) > 0 || len(prs) > 0 {
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
		if platforms[in.Window] == "" {
			errs = append(errs, s.openPlatform(in.Window, in.Track))
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
// open comments, how many relay messages the user hasn't seen, and
// whether @github's events wait for a held pointer, which Collect
// messages sends.
type Signals struct {
	PRs      []PRSignal
	RelayNew int
	Held     bool
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
	return Signals{PRs: prs, RelayNew: n, Held: s.heldFrom(id, relay.GitHubHandle)}, nil
}

// trainsKey is what of trains a Platform shows, to tell a change.
func trainsKey(trains []train.Train) string {
	var b strings.Builder
	for _, t := range trains {
		fmt.Fprintf(&b, "%s %s %s %s %v|", t.Handle, t.Engine, t.Status.ID, t.Outcome.ID, t.Held)
	}
	return b.String()
}

// openPlatform opens track id's Platform in window, unless it has one.
func (s *Service) openPlatform(window, id string) error {
	s.platformMu.Lock()
	defer s.platformMu.Unlock()
	platforms, err := s.Windows.Platforms()
	if err != nil || platforms[window] != "" {
		return err
	}
	return s.Windows.OpenPlatform(window, s.platformCommand(id))
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

func (w TmuxWindows) FitPlatforms() error {
	panes, err := w.Tmux.ListSessionPanes(w.Session)
	if err != nil {
		return err
	}
	var errs []error
	for _, pane := range trackwin.Unfit(panes) {
		errs = append(errs, w.Tmux.ResizePaneWidth(pane, trackwin.PlatformWidth))
	}
	return errors.Join(errs...)
}

// OpenPlatform kills a pane it opened but couldn't label: unlabelled,
// it wouldn't be found as the Platform, and the next tick would open
// another.
func (w TmuxWindows) OpenPlatform(window, command string) error {
	pane, err := trackwin.OpenPlatform(w.Tmux, window, "", trackwin.Process{Command: command})
	if err != nil && pane != "" {
		_ = w.Tmux.KillPane(pane)
	}
	return err
}
