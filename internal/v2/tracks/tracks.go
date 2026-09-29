// Package tracks creates, ends, resumes and cleans tracks: the steps
// from the New track form to an agent running in the track's window,
// and undoing them when one fails.
package tracks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
	"github.com/bluegardenproject/tracks/internal/v2/workspace"
)

// Problem is why a request can't be done, worded for the user.
type Problem string

func (p Problem) Error() string { return string(p) }

// ErrNoEngine is Create's problem while no engine is added.
const ErrNoEngine Problem = "Add an engine on the Engines tab first."

// Store is what tracks keeps in the database.
type Store interface {
	Repos(ctx context.Context) ([]store.Repo, error)
	AddTrack(ctx context.Context, t track.Track) error
	Track(ctx context.Context, id string) (track.Track, error)
	OpenTracks(ctx context.Context) ([]track.Track, error)
	EndedTracks(ctx context.Context, limit int) ([]track.Track, error)
	CloseTrack(ctx context.Context, id string, at time.Time) error
	ReopenTrack(ctx context.Context, id, name string) error
	CleanTrack(ctx context.Context, id string, at time.Time) error
	SetBranch(ctx context.Context, id string, position int, branch string) error
}

// Worktrees makes and removes a track's worktrees.
type Worktrees interface {
	Add(ctx context.Context, t track.Track, progress func(string)) ([]track.Repo, error)
	// Remove undoes Add, branches included.
	Remove(ctx context.Context, t track.Track) error
	Restore(ctx context.Context, t track.Track, progress func(string)) ([]track.Repo, error)
	Unsaved(ctx context.Context, t track.Track) ([]workspace.Unsaved, error)
	// Branches are t's repos on the branches their worktrees are on.
	Branches(ctx context.Context, t track.Track) []track.Repo
	// RemoveWorktrees removes the worktrees and keeps the branches.
	RemoveWorktrees(ctx context.Context, id string, repos []track.Repo) error
}

// Windows opens and closes the tracks' windows.
type Windows interface {
	List() ([]trackwin.Info, error)
	Open(s trackwin.Spec) (trackwin.Window, error)
	Close(window string) error
}

// Service creates, lists and ends tracks.
type Service struct {
	Store     Store
	Worktrees Worktrees
	Windows   Windows
	// Settings reads the Engines tab's settings, for each creation.
	Settings func() (settings.Settings, error)
	// Engines are the engines by ID; nil is the real ones.
	Engines map[string]Engine
	// SocketDir and BinDir go into every agent pane's environment.
	SocketDir, BinDir string
	// Now and NewID are the clock and the track IDs; nil is the real
	// ones.
	Now   func() time.Time
	NewID func() string

	mu sync.Mutex
	// claimed are the window names of tracks being created or resumed,
	// whose windows don't exist yet.
	claimed map[string]bool
	// busy are the tracks being resumed, cleaned or ended.
	busy map[string]bool
}

// ended is how many ended tracks List returns, the most recent ones.
const ended = 100

// hold claims track id for one Resume, Clean or End at a time, until
// release.
func (s *Service) hold(ctx context.Context, id string) (t track.Track, release func(), err error) {
	if t, err = s.Store.Track(ctx, id); errors.Is(err, store.ErrNotFound) {
		return t, nil, Problem("That track is gone.")
	} else if err != nil {
		return t, nil, err
	}
	s.mu.Lock()
	taken := s.busy[id]
	if !taken {
		if s.busy == nil {
			s.busy = map[string]bool{}
		}
		s.busy[id] = true
	}
	s.mu.Unlock()
	if taken {
		return t, nil, Problem(t.Name + " is busy.")
	}
	release = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.busy, id)
	}
	// Read again: what the holder before changed is saved by now.
	if t, err = s.Store.Track(ctx, id); err != nil {
		release()
		return t, nil, err
	}
	return t, release, nil
}

func (s *Service) isBusy(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy[id]
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) newID() string {
	if s.NewID != nil {
		return s.NewID()
	}
	return NewID(s.now())
}

// NewID is a track ID made at: YYYYMMDD-HHMMSS-<6 hex>, in UTC, as v1
// makes them. They sort by time and are readable.
func NewID(at time.Time) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	return at.UTC().Format("20060102-150405") + "-" + hex.EncodeToString(b)
}

// claimName picks the window name of t and holds it until release, so
// two tracks created at once don't get the same one.
func (s *Service) claimName(t track.Track) (name string, release func(), err error) {
	infos, err := s.Windows.List()
	if err != nil {
		return "", nil, err
	}
	names := map[string]bool{}
	for _, in := range infos {
		names[in.Name] = true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.claimed == nil {
		s.claimed = map[string]bool{}
	}
	label := track.WindowLabel(t.Name, t.Document, t.Prompt)
	name = track.WindowName(label, t.ID, func(n string) bool { return names[n] || s.claimed[n] })
	s.claimed[name] = true
	return name, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.claimed, name)
	}, nil
}

// TmuxWindows are the track windows of a tmux session.
type TmuxWindows struct {
	Tmux interface {
		trackwin.Tmux
		KillWindow(window string) error
	}
	Session string
}

func (w TmuxWindows) List() ([]trackwin.Info, error) { return trackwin.List(w.Tmux, w.Session) }

func (w TmuxWindows) Open(s trackwin.Spec) (trackwin.Window, error) {
	return trackwin.Open(w.Tmux, w.Session, s)
}

func (w TmuxWindows) Close(window string) error { return w.Tmux.KillWindow(window) }
