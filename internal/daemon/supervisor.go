package daemon

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/bluegardenproject/tracks/internal/claude"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

// supervisor wraps one running Claude session for one track.
// Unlike the original child-process model, the process itself is
// owned by tmux: we tracked it down via the pane_pid returned when
// the window was opened, and we watch it with kill(pid, 0) +
// `tmux has-window` checks.
//
// Owning the process via tmux is what lets the user *interact*
// with Claude in the track window. A daemon-owned child process
// would have no TTY and no way for the user to type into it.
type supervisor struct {
	trackID    string
	windowName string
	pid        int
	cancel     context.CancelFunc
	done       chan struct{}

	// lastPane is the most recent capture-pane snapshot. Used to
	// detect a stalled pane (= Claude waiting for user input).
	lastPane         string
	lastPaneChangeAt time.Time
}

// windowNameFor returns the tmux window name for a track. Kept here
// (also duplicated in cmd/attach.go for the CLI side) because both
// daemon and CLI need to agree on it.
func windowNameFor(trackID string) string {
	if len(trackID) >= 6 {
		return "t-" + trackID[len(trackID)-6:]
	}
	return "t-" + trackID
}

// startSupervisor opens a tmux window for the track with claude
// running inside it and starts the watcher goroutines.
func (s *Server) startSupervisor(ctx context.Context, t state.Track) (*supervisor, error) {
	opts, err := claude.BuildOptions(s.cfg, t, s.socketDir)
	if err != nil {
		return nil, err
	}
	tm := tmux.New()
	window := windowNameFor(t.ID)
	pid, err := tm.NewWindowReturningPaneID(s.cfg.Tmux.SessionName, window, opts.ShellCommand(), opts.CWD)
	if err != nil {
		return nil, fmt.Errorf("open tmux window: %w", err)
	}

	supCtx, cancel := context.WithCancel(ctx)
	sup := &supervisor{
		trackID:          t.ID,
		windowName:       window,
		pid:              pid,
		cancel:           cancel,
		done:             make(chan struct{}),
		lastPaneChangeAt: time.Now(),
	}

	// Persist the live state.
	t.Status = state.StatusRunning
	t.PID = pid
	if err := s.store.Put(t); err != nil {
		// We've already opened the window. Close it and bail —
		// otherwise the daemon would be orphaned from the truth.
		_ = tm.KillWindow(s.cfg.Tmux.SessionName, window)
		cancel()
		return nil, fmt.Errorf("persist running state: %w", err)
	}

	s.mu.Lock()
	if s.supervisors == nil {
		s.supervisors = make(map[string]*supervisor)
	}
	s.supervisors[t.ID] = sup
	s.mu.Unlock()

	go s.watchTrackProcess(supCtx, sup)
	return sup, nil
}

// watchTrackProcess polls the pane pid until it stops being a live
// process, and watches the pane's rendered contents to detect when
// Claude is sitting at its prompt waiting for user input.
//
//   - pid dead OR window gone → Done (or Errored — we don't have
//     a reliable exit-code source from tmux here).
//   - pane content unchanged for paneIdleThreshold → Waiting.
//   - pane content changed within paneIdleThreshold → Running.
func (s *Server) watchTrackProcess(ctx context.Context, sup *supervisor) {
	defer close(sup.done)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	tm := tmux.New()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			alive := processAlive(sup.pid)
			windowExists, _ := tm.HasWindow(s.cfg.Tmux.SessionName, sup.windowName)
			if !alive || !windowExists {
				s.finalizeTrack(sup.trackID)
				s.mu.Lock()
				delete(s.supervisors, sup.trackID)
				s.mu.Unlock()
				return
			}
			s.refreshRunningStatus(tm, sup)
		}
	}
}

// paneIdleThreshold is how long the pane must be unchanged before
// we flip the track to Waiting. Short enough that the dashboard
// reflects "Claude wants you" within a few seconds; long enough
// that a brief thinking pause doesn't flicker.
const paneIdleThreshold = 6 * time.Second

// refreshRunningStatus captures the pane content and updates the
// stored track to Running or Waiting based on whether the snapshot
// changed since the last poll. Errors from capture-pane are
// swallowed — they shouldn't bring down the supervisor.
func (s *Server) refreshRunningStatus(tm *tmux.Client, sup *supervisor) {
	snapshot, err := tm.CapturePane(s.cfg.Tmux.SessionName, sup.windowName)
	if err != nil {
		return
	}
	if snapshot != sup.lastPane {
		sup.lastPane = snapshot
		sup.lastPaneChangeAt = time.Now()
	}
	t, ok := s.store.Get(sup.trackID)
	if !ok || t.Status.IsTerminal() {
		return
	}
	idle := time.Since(sup.lastPaneChangeAt) > paneIdleThreshold
	switch {
	case idle && t.Status != state.StatusWaiting:
		t.Status = state.StatusWaiting
		_ = s.store.Put(t)
	case !idle && t.Status != state.StatusRunning:
		t.Status = state.StatusRunning
		_ = s.store.Put(t)
	}
}

// finalizeTrack writes the terminal status for trackID if it isn't
// already terminal.
func (s *Server) finalizeTrack(trackID string) {
	t, ok := s.store.Get(trackID)
	if !ok || t.Status.IsTerminal() {
		return
	}
	now := time.Now().UTC()
	t.ExitedAt = &now
	t.Status = state.StatusDone
	_ = s.store.Put(t)
}

// Stop signals the supervisor to wind down by killing the tmux
// window (which SIGHUPs claude). Waits for the watcher to see the
// disappearance and finalize.
func (sup *supervisor) Stop(sessionName string) {
	if sup == nil {
		return
	}
	tm := tmux.New()
	_ = tm.KillWindow(sessionName, sup.windowName)
	select {
	case <-sup.done:
	case <-time.After(5 * time.Second):
	}
}

// Kill is harsher: SIGKILL the pid directly, then kill the window
// for good measure.
func (sup *supervisor) Kill(sessionName string) {
	if sup == nil {
		return
	}
	if sup.pid > 0 {
		_ = syscall.Kill(sup.pid, syscall.SIGKILL)
	}
	tm := tmux.New()
	_ = tm.KillWindow(sessionName, sup.windowName)
	select {
	case <-sup.done:
	case <-time.After(2 * time.Second):
	}
}

// stopAllSupervisors closes every active track window. Called from
// Server.Stop when the daemon is winding down. SIGTERMs in parallel
// so a slow shutdown doesn't hold the others up.
func (s *Server) stopAllSupervisors() {
	s.mu.Lock()
	sups := make([]*supervisor, 0, len(s.supervisors))
	for _, sup := range s.supervisors {
		sups = append(sups, sup)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	for _, sup := range sups {
		wg.Add(1)
		go func(sp *supervisor) {
			defer wg.Done()
			sp.Stop(s.cfg.Tmux.SessionName)
		}(sup)
	}
	wg.Wait()
}

