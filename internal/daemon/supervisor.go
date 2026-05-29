package daemon

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/bluegardenproject/tracks/internal/claude"
	"github.com/bluegardenproject/tracks/internal/state"
)

// supervisor wraps one running Claude process for one track. It owns
// the goroutines that watch the process and tail the log, and is
// responsible for transitioning the track's status from
// Pending → Running → (Waiting) → Done/Errored as time passes.
type supervisor struct {
	trackID string
	cmd     *exec.Cmd
	cancel  context.CancelFunc

	done chan struct{}
}

// startSupervisor launches Claude for the given track and starts the
// log-tailing + process-waiting goroutines. Returns the supervisor
// for later Stop() / lookup.
func (s *Server) startSupervisor(ctx context.Context, t state.Track) (*supervisor, error) {
	opts, err := claude.BuildOptions(s.cfg, t, s.socketDir)
	if err != nil {
		return nil, err
	}
	cmd, err := claude.Spawn(opts)
	if err != nil {
		return nil, err
	}

	supCtx, cancel := context.WithCancel(ctx)
	sup := &supervisor{
		trackID: t.ID,
		cmd:     cmd,
		cancel:  cancel,
		done:    make(chan struct{}),
	}

	// Mark Running with PID.
	t.Status = state.StatusRunning
	t.PID = cmd.Process.Pid
	if err := s.store.Put(t); err != nil {
		// We've already started the process. Killing it on a
		// persistence failure is the safer choice — otherwise the
		// daemon is orphaned from the truth and the user can't
		// reach the track via `tracks ls`.
		_ = cmd.Process.Kill()
		cancel()
		return nil, fmt.Errorf("persist running state: %w", err)
	}

	s.mu.Lock()
	if s.supervisors == nil {
		s.supervisors = make(map[string]*supervisor)
	}
	s.supervisors[t.ID] = sup
	s.mu.Unlock()

	// Log-tail goroutine.
	events := make(chan claude.Event, 32)
	go func() {
		_ = claude.TailLog(supCtx, t.LogPath, events)
		close(events)
	}()

	// Event consumer goroutine — translates events into state writes.
	go s.consumeEvents(supCtx, t.ID, events)

	// Process-wait goroutine — when Claude exits, mark Done/Errored.
	go s.waitProcess(sup, t.ID)

	return sup, nil
}

// consumeEvents updates the track's state row in response to log events.
func (s *Server) consumeEvents(ctx context.Context, trackID string, events <-chan claude.Event) {
	idleTicker := time.NewTicker(15 * time.Second)
	defer idleTicker.Stop()
	lastActivity := time.Now()

	flushIdle := func() {
		t, ok := s.store.Get(trackID)
		if !ok || t.Status.IsTerminal() {
			return
		}
		// If no event for >60s and process is still alive, mark Waiting.
		if time.Since(lastActivity) > 60*time.Second && t.Status == state.StatusRunning {
			t.Status = state.StatusWaiting
			_ = s.store.Put(t)
		}
		// Conversely, return to Running if we just saw activity.
		if time.Since(lastActivity) < 5*time.Second && t.Status == state.StatusWaiting {
			t.Status = state.StatusRunning
			_ = s.store.Put(t)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-idleTicker.C:
			flushIdle()
		case ev, ok := <-events:
			if !ok {
				return
			}
			lastActivity = time.Now()
			switch e := ev.(type) {
			case claude.PRMarker:
				if t, ok := s.store.Get(trackID); ok {
					t.PRURL = e.URL
					_ = s.store.Put(t)
				}
			default:
				// AssistantText / ToolUse currently don't mutate
				// persisted state — they're surfaced to the per-track
				// tmux pane via tail -F. lastActivity above is what
				// the Running/Waiting heuristic needs.
				_ = e
			}
		}
	}
}

// waitProcess blocks on cmd.Wait() and writes the terminal state.
func (s *Server) waitProcess(sup *supervisor, trackID string) {
	defer close(sup.done)
	err := sup.cmd.Wait()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}
	t, found := s.store.Get(trackID)
	if !found {
		return
	}
	if !t.Status.IsTerminal() {
		now := time.Now().UTC()
		t.ExitedAt = &now
		t.ExitCode = &exitCode
		if exitCode == 0 {
			t.Status = state.StatusDone
		} else {
			t.Status = state.StatusErrored
		}
		_ = s.store.Put(t)
	}
	// Remove from active supervisor map.
	s.mu.Lock()
	delete(s.supervisors, trackID)
	s.mu.Unlock()
}

// Stop sends SIGTERM, waits up to 5s, then SIGKILL.
func (sup *supervisor) Stop() {
	if sup == nil || sup.cmd == nil || sup.cmd.Process == nil {
		return
	}
	_ = sup.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-sup.done:
		return
	case <-time.After(5 * time.Second):
	}
	_ = sup.cmd.Process.Signal(syscall.SIGKILL)
	<-sup.done
}

// Kill is Stop with no SIGTERM grace.
func (sup *supervisor) Kill() {
	if sup == nil || sup.cmd == nil || sup.cmd.Process == nil {
		return
	}
	_ = sup.cmd.Process.Signal(syscall.SIGKILL)
	<-sup.done
}

// stopAllSupervisors is invoked when the daemon itself is shutting
// down. We SIGTERM everyone in parallel and wait briefly.
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
			sp.Stop()
		}(sup)
	}
	wg.Wait()
}

