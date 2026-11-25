// Package tracks creates, ends, resumes and cleans tracks: the steps
// from the New track form to an agent running in the track's window,
// and undoing them when one fails.
package tracks

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/procs"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/workspace"
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
	InterruptedTracks(ctx context.Context) ([]track.Track, error)
	SideTracks(ctx context.Context, id string) ([]track.Track, error)
	FilteredTracks(ctx context.Context, f track.Filter, now time.Time, limit int) ([]track.Track, error)
	Filter(ctx context.Context) (track.Filter, error)
	SetFilter(ctx context.Context, f track.Filter) error
	SetState(ctx context.Context, id string, st track.State) error
	DeleteTrack(ctx context.Context, id string) error
	Rename(ctx context.Context, id, name string) error
	SetTitle(ctx context.Context, id, title string) error
	SetBranch(ctx context.Context, id string, position int, branch string) error
	SetSetupDone(ctx context.Context, id, name string, done bool) error
	AddTrackError(ctx context.Context, id string, e track.Failure) error
	ClearTrackErrors(ctx context.Context, id, kind, subject string) (bool, error)
	ClaimPorts(ctx context.Context, id string, first, size, blocks int) (int, error)
	ProxyPorts(ctx context.Context) ([]store.ProxyPort, error)
	AddProxyPort(ctx context.Context, port int) error
	RemoveProxyPort(ctx context.Context, port int) error
	SetProxyInput(ctx context.Context, port int, in store.ProxyInput) error
	AddTrackRepo(ctx context.Context, id string, r track.Repo) error
	Promote(ctx context.Context, t track.Track) error
	AddPR(ctx context.Context, id string, pr track.PR, at time.Time) (bool, error)
	SavePR(ctx context.Context, id string, pr track.PR, at time.Time) (track.PRState, error)
	UnsettledPRs(ctx context.Context) ([]store.TrackPR, error)
	SavePRHealth(ctx context.Context, id, url string, h track.Health) (track.Health, error)
	SavePRThreads(ctx context.Context, id, url string, n int) (int, bool, error)
	AddInstruction(ctx context.Context, in store.Instruction) error
	WaitingInstructions(ctx context.Context) ([]store.Instruction, error)
	InstructionTyped(ctx context.Context, id int64, at time.Time) error
	DropInstructions(ctx context.Context, pr string, kinds ...string) error
	InstructionWaiting(ctx context.Context, id int64) (bool, error)
	SaveDraft(ctx context.Context, d store.Draft) error
	Drafts(ctx context.Context) ([]store.Draft, error)
	Draft(ctx context.Context, id string) (store.Draft, error)
	DeleteDraft(ctx context.Context, id string) (bool, error)
	AddRelayMessage(ctx context.Context, id string, m relay.Message) (relay.Message, error)
	RelayUnread(ctx context.Context, id, reader string) ([]relay.Message, error)
	MarkRelayRead(ctx context.Context, id, reader string, upTo int64) (bool, error)
	RelayMessage(ctx context.Context, id string, msg int64) (relay.Message, error)
	RelayLog(ctx context.Context, id string) ([]relay.Message, error)
	RelayUnseen(ctx context.Context, id, reader string) (int, error)
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
	// EndHidden closes track's trains, such as its reviewer, once its
	// window is gone some other way than Close.
	EndHidden(track string) error
	// Attention marks window as needing the user, or not.
	Attention(window string, on bool) error
	// Screen is what window's agent pane shows.
	Screen(window string) (string, error)
	// Respawn restarts window's agent on s, keeping the window.
	Respawn(window string, s trackwin.Spec) (trackwin.Window, error)
	// Buttons shows buttons, a tmux format, on window for the footer.
	Buttons(window, buttons string) error
	// Prompt types text into window's agent pane and submits it.
	Prompt(window, text string) error
	// Info shows text on window, for the footer's top row.
	Info(window, text string) error
	// LastKey is when a client showing window last had a key, the zero
	// time when none shows it.
	LastKey(window string) (time.Time, error)
	// StyledScreen is what window's agent pane shows, with its styles.
	StyledScreen(window string) (string, error)
	// Paste pastes text into window's agent pane and submits it.
	Paste(window, text string) error
	// Scrolled reports whether window's agent pane is in a mode, such as
	// copy mode while the user scrolls back.
	Scrolled(window string) (bool, error)
	// LeaveScroll takes window's agent pane out of its mode, such as
	// copy mode.
	LeaveScroll(window string) error
	// Platforms are the track windows' Platform panes, by window;
	// OpenPlatform opens window's running command.
	Platforms() (map[string]string, error)
	OpenPlatform(window, command string) error
	// FitPlatforms puts each Platform back to its width, which tmux
	// changes as it scales the panes with their window.
	FitPlatforms() error
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
	// Socket is the daemon's socket, which an agent's sandbox has to
	// let its tracks commands reach.
	Socket string
	// HooksDir holds each track's hooks, a folder per track; "" starts
	// agents without them.
	HooksDir string
	// Reviewers runs the tracks' reviewers; nil runs none.
	Reviewers ReviewerPanes
	// Setups runs the repos' setups; nil runs none.
	Setups SetupPanes
	// Servers runs the repos' dev servers; nil runs none.
	Servers ServerPanes
	// Snapshot reads the processes and the ports they listen on; nil
	// asks the system.
	Snapshot func(ctx context.Context) (procs.Snapshot, error)
	// PortFree reports whether nothing listens on a port; nil asks the
	// system.
	PortFree func(port int) bool
	// CopyEnv copies a primary checkout's .env files into a new
	// worktree; nil is workspace.CopyEnv.
	CopyEnv func(ctx context.Context, primary, worktree string) ([]string, error)
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
	// makingSides counts the side tracks being made for each main track,
	// which can't end meanwhile.
	makingSides map[string]int
	// reviewMu keeps a review's start, which resets the review's folder,
	// and a reviewer's reply, which writes to it, apart.
	reviewMu sync.Mutex
	// platformMu keeps a window to one Platform while a new window's
	// and the tick's open it.
	platformMu sync.Mutex
	// setupMu keeps one StartSetup looking at and opening panes at a
	// time, so a setup never starts twice.
	setupMu sync.Mutex
	// serverMu keeps one Up or Down looking at and changing panes at a
	// time, so a server never starts twice.
	serverMu sync.Mutex
	// reporting keeps one Report reading and writing a state at a time.
	reporting sync.Mutex
	// gone counts, per waiting track, the checks in a row that found
	// its dialog closed.
	gone map[string]int
	// windows are the track windows the last Sweep saw; only Sweep
	// uses them.
	windows []trackwin.Info
	// comments are the open tracks' review threads, as CountComments
	// last counted them and RunAction sent them.
	comments map[string]map[string]actions.Comments // by track, then PR URL
	// threads are the open review threads on each PR, as the last poll
	// read them, by URL; repoNames the GitHub repo of each repo's
	// checkout, by its path, as gh resolved it.
	threads   map[string]int
	repoNames map[string]string
	// badges are the badges last written into each train's overlay
	// header, by session.
	badges map[string]string
	// trainTold is the review run, by track, whose reviewer the user
	// was told waits on them.
	trainTold map[string]string
	// info is what SyncInfo last showed, by window.
	info map[string]string
	// turns says, by track, whether its agent is in a turn, from its
	// hooks; a track it hasn't heard of may be.
	turns map[string]bool
	// pointing is the pointer typed into each track's agent that waits
	// for its turn; pointed how far each track was pointed to on its
	// relay; retryAt when a pointer the agent didn't take may go again.
	pointing   map[string]pointing
	pointed    map[string]int64
	retryAt    map[string]time.Time
	pointerGen uint64
	// held is, by track, whose messages wait while its pointer is held
	// back.
	held map[string]map[string]bool
	// trainsSeen is, by track, what SyncTrains last saw of its trains.
	trainsSeen map[string]string
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
	label := track.WindowLabel(t.Name, t.Document, cmp.Or(reviewLabel(t.ReviewRef), t.Prompt))
	name = track.WindowName(label, t.ID, func(n string) bool { return names[n] || s.claimed[n] })
	s.claimed[name] = true
	return name, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.claimed, name)
	}, nil
}

// reviewLabel is what a review track is named after when it has no
// name: the PR or branch ref points at, "" when ref doesn't parse.
func reviewLabel(ref string) string {
	r, err := workspace.ParseReview(ref)
	if err != nil {
		return ""
	}
	return "Review " + r.Label
}

// TmuxWindows are the track windows of a tmux session.
type TmuxWindows struct {
	Tmux interface {
		trackwin.TrainTmux
		RefreshStatus(client string) error
		ListSessionPanes(session string) ([]tmux.Pane, error)
		ResizePaneWidth(pane string, cols int) error
		KillPane(pane string) error
		KillWindow(window string) error
		CapturePane(pane string) (string, error)
		CapturePaneStyled(pane string) (string, error)
		LastKey(window string) (time.Time, error)
		InMode(pane string) (bool, error)
		CancelMode(pane string) error
		CaptureHistory(pane string, lines int) (string, error)
	}
	Session string
}

func (w TmuxWindows) List() ([]trackwin.Info, error) { return trackwin.List(w.Tmux, w.Session) }

func (w TmuxWindows) Open(s trackwin.Spec) (trackwin.Window, error) {
	return trackwin.Open(w.Tmux, w.Session, s)
}

// Close stops window's dev servers, then closes it and the track's
// trains, such as its reviewer, even when a server wouldn't stop.
func (w TmuxWindows) Close(window string) error {
	stopped := w.StopServers(window)
	id, trains := trackwin.TrackOf(w.Tmux, window)
	if id != "" {
		trains = trackwin.CloseTrains(w.Tmux, id, "")
	}
	return errors.Join(stopped, w.Tmux.KillWindow(window), trains)
}

// EndHidden closes track's trains, its window gone.
func (w TmuxWindows) EndHidden(track string) error {
	return trackwin.CloseTrains(w.Tmux, track, "")
}

func (w TmuxWindows) OpenReviewer(s trackwin.TrainSpec) error {
	_, err := trackwin.OpenTrain(w.Tmux, s)
	return err
}

func (w TmuxWindows) Reviewer(track string) (ReviewerState, error) {
	p, err := trackwin.FindTrain(w.Tmux, track, trackwin.RoleReviewer)
	if errors.Is(err, trackwin.ErrNoTrain) {
		return ReviewerState{}, nil
	} else if err != nil {
		return ReviewerState{}, err
	}
	attached, err := trackwin.Attached(w.Tmux, p.Session)
	return ReviewerState{Pane: p.ID, Session: p.Session, Exited: p.Dead, Attached: attached}, err
}

func (w TmuxWindows) Reviewers() (map[string]ReviewerState, error) {
	panes, err := trackwin.Trains(w.Tmux, "")
	if err != nil {
		return nil, err
	}
	return reviewerStates(panes), nil
}

func (w TmuxWindows) TrainBadge(session, badges string) error {
	if err := w.Tmux.SetSessionOption(session, trackwin.TrainBadgeOption, badges); err != nil {
		return err
	}
	clients, err := w.Tmux.SessionClients(session)
	for _, c := range clients {
		err = errors.Join(err, w.Tmux.RefreshStatus(c))
	}
	return err
}

func (w TmuxWindows) TypeReviewer(pane, text string) error { return trackwin.Type(w.Tmux, pane, text) }

func (w TmuxWindows) CaptureReviewer(pane string) (string, error) { return w.Tmux.CapturePane(pane) }

func (w TmuxWindows) RespawnReviewer(pane, dir, command string) error {
	return w.Tmux.RespawnPane(pane, dir, command)
}

func (w TmuxWindows) CloseReviewer(track string) error {
	return trackwin.CloseTrains(w.Tmux, track, trackwin.RoleReviewer)
}

// reviewerStates are the tracks' reviewers among the trains' panes.
func reviewerStates(panes []tmux.Pane) map[string]ReviewerState {
	out := map[string]ReviewerState{}
	for _, p := range panes {
		if p.Role == trackwin.RoleReviewer && p.Track != "" {
			out[p.Track] = ReviewerState{Pane: p.ID, Session: p.Session, Exited: p.Dead}
		}
	}
	return out
}

func (w TmuxWindows) Panes(window string) ([]tmux.Pane, error) { return w.Tmux.ListPanes(window) }

func (w TmuxWindows) AddSetup(window, dir string, p trackwin.Process) error {
	_, err := trackwin.AddSetup(w.Tmux, window, dir, p)
	return err
}

func (w TmuxWindows) ClosePane(pane string) error { return w.Tmux.KillPane(pane) }

func (w TmuxWindows) Respawn(window string, s trackwin.Spec) (trackwin.Window, error) {
	return trackwin.Respawn(w.Tmux, window, s)
}

func (w TmuxWindows) Attention(window string, on bool) error {
	return trackwin.SetAttention(w.Tmux, window, on)
}

func (w TmuxWindows) Buttons(window, buttons string) error {
	return trackwin.SetButtons(w.Tmux, window, buttons)
}

func (w TmuxWindows) Info(window, text string) error {
	return trackwin.SetInfo(w.Tmux, window, text)
}

func (w TmuxWindows) Prompt(window, text string) error {
	return trackwin.Prompt(w.Tmux, window, text)
}

func (w TmuxWindows) Screen(window string) (string, error) {
	return w.agentScreen(window, w.Tmux.CapturePane)
}

func (w TmuxWindows) StyledScreen(window string) (string, error) {
	return w.agentScreen(window, w.Tmux.CapturePaneStyled)
}

// agentScreen is window's agent pane as capture captures it.
func (w TmuxWindows) agentScreen(window string, capture func(pane string) (string, error)) (string, error) {
	pane, err := w.agentPane(window)
	if err != nil {
		return "", err
	}
	return capture(pane)
}

func (w TmuxWindows) LastKey(window string) (time.Time, error) { return w.Tmux.LastKey(window) }

func (w TmuxWindows) Paste(window, text string) error { return trackwin.Paste(w.Tmux, window, text) }

func (w TmuxWindows) LeaveScroll(window string) error {
	pane, err := w.agentPane(window)
	if err != nil {
		return err
	}
	return w.Tmux.CancelMode(pane)
}

// agentPane is window's agent pane.
func (w TmuxWindows) agentPane(window string) (string, error) {
	panes, err := w.Tmux.ListPanes(window)
	if err != nil {
		return "", err
	}
	for _, p := range panes {
		if p.Role == trackwin.RoleAgent {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("window %s has no agent pane", window)
}

func (w TmuxWindows) Scrolled(window string) (bool, error) {
	pane, err := w.agentPane(window)
	if err != nil {
		return false, err
	}
	return w.Tmux.InMode(pane)
}
