// Package tracks creates, ends, resumes and cleans tracks: the steps
// from the New track form to an agent running in the track's window,
// and undoing them when one fails.
package tracks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
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
	EndedBefore(ctx context.Context, at time.Time) ([]track.Track, error)
	FilteredTracks(ctx context.Context, f track.Filter, now time.Time, limit int) ([]track.Track, error)
	Filter(ctx context.Context) (track.Filter, error)
	SetFilter(ctx context.Context, f track.Filter) error
	SetState(ctx context.Context, id string, st track.State) error
	DeleteTrack(ctx context.Context, id string) error
	Rename(ctx context.Context, id, name string) error
	SetCost(ctx context.Context, id string, cost float64) error
	SetBranch(ctx context.Context, id string, position int, branch string) error
	AddTrackRepo(ctx context.Context, id string, r track.Repo) error
	Promote(ctx context.Context, t track.Track) error
	AddPR(ctx context.Context, id string, pr track.PR, at time.Time) (bool, error)
	SavePR(ctx context.Context, id string, pr track.PR, at time.Time) (track.PRState, error)
	UnsettledPRs(ctx context.Context) ([]store.TrackPR, error)
	SaveDraft(ctx context.Context, d store.Draft) error
	Drafts(ctx context.Context) ([]store.Draft, error)
	Draft(ctx context.Context, id string) (store.Draft, error)
	DeleteDraft(ctx context.Context, id string) (bool, error)
}

// Worktrees makes and removes a track's worktrees.
type Worktrees interface {
	Add(ctx context.Context, t track.Track, progress func(string)) ([]track.Repo, error)
	// AddRepo makes r's worktree for work track id on branch.
	AddRepo(ctx context.Context, id string, r track.Repo, branch string, progress func(string)) (track.Repo, error)
	// Remove undoes Add, branches included.
	Remove(ctx context.Context, t track.Track) error
	// Missing are t's repos whose worktree is gone, which Restore
	// re-creates.
	Missing(t track.Track) []track.Repo
	Restore(ctx context.Context, t track.Track, progress func(string)) ([]track.Repo, error)
	// Branches are t's repos on the branches their worktrees are on.
	Branches(ctx context.Context, t track.Track) []track.Repo
	// RemoveWorktrees removes the worktrees and keeps the branches.
	RemoveWorktrees(ctx context.Context, id string, repos []track.Repo) error
	// Lost is what Discard would lose; Discard removes the worktrees
	// and a work track's local branches.
	Lost(ctx context.Context, t track.Track) ([]workspace.Unsaved, error)
	Discard(ctx context.Context, t track.Track) error
}

// Windows opens and closes the tracks' windows.
type Windows interface {
	List() ([]trackwin.Info, error)
	Open(s trackwin.Spec) (trackwin.Window, error)
	Close(window string) error
	// Attention marks window as needing the user, or not.
	Attention(window string, on bool) error
	// Screen is what window's agent pane shows.
	Screen(window string) (string, error)
	// Respawn restarts window's agent on s, keeping the window.
	Respawn(window string, s trackwin.Spec) (trackwin.Window, error)
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
	// HooksDir holds each track's hooks, a folder per track; "" starts
	// agents without them.
	HooksDir string
	// GitHub is asked about the tracks' PRs; nil doesn't poll.
	GitHub GitHub
	// Changes hears when the track windows change, which Store doesn't
	// see; give Store as Watched with the same Changes for the rest.
	Changes *Changes
	// Notify hears the notices: a track needing the user or failing,
	// its PRs opening or settling. nil tells nobody.
	Notify func(Notice)
	// Now and NewID are the clock and the track IDs; nil is the real
	// ones.
	Now   func() time.Time
	NewID func() string

	mu sync.Mutex
	// claimed are the window names of tracks being created or resumed,
	// whose windows don't exist yet.
	claimed map[string]bool
	// busy are the tracks being resumed, archived or ended.
	busy map[string]bool
	// reporting keeps one Report reading and writing a state at a time.
	reporting sync.Mutex
	// gone counts, per waiting track, the checks in a row that found
	// its dialog closed.
	gone map[string]int
	// windows are the track windows the last Sweep saw; only Sweep
	// uses them.
	windows []trackwin.Info
	// transcripts are the signatures of the open Claude tracks'
	// transcripts the last Costs read; only Costs uses them.
	transcripts map[string]string
}

// ended is how many ended tracks List returns, the most recent ones.
const ended = 100

// hold claims track id for one Resume, Archive or End at a time, until
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
		CapturePane(pane string) (string, error)
	}
	Session string
}

func (w TmuxWindows) List() ([]trackwin.Info, error) { return trackwin.List(w.Tmux, w.Session) }

func (w TmuxWindows) Open(s trackwin.Spec) (trackwin.Window, error) {
	return trackwin.Open(w.Tmux, w.Session, s)
}

func (w TmuxWindows) Close(window string) error { return w.Tmux.KillWindow(window) }

func (w TmuxWindows) Respawn(window string, s trackwin.Spec) (trackwin.Window, error) {
	return trackwin.Respawn(w.Tmux, window, s)
}

func (w TmuxWindows) Attention(window string, on bool) error {
	return trackwin.SetAttention(w.Tmux, window, on)
}

func (w TmuxWindows) Screen(window string) (string, error) {
	panes, err := w.Tmux.ListPanes(window)
	if err != nil {
		return "", err
	}
	for _, p := range panes {
		if p.Role == trackwin.RoleAgent {
			return w.Tmux.CapturePane(p.ID)
		}
	}
	return "", fmt.Errorf("window %s has no agent pane", window)
}
