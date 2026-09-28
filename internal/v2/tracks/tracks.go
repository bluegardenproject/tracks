// Package tracks creates and ends tracks: the steps from the New track
// form to an agent running in the track's window, and undoing them
// when one fails.
package tracks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
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
	OpenTracks(ctx context.Context) ([]track.Track, error)
	CloseTrack(ctx context.Context, id string, at time.Time) error
}

// Worktrees makes and removes a track's worktrees.
type Worktrees interface {
	Add(ctx context.Context, t track.Track, progress func(string)) ([]track.Repo, error)
	Remove(ctx context.Context, t track.Track) error
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
	// claimed are the window names of tracks being created, whose
	// windows don't exist yet.
	claimed map[string]bool
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
