package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/services"
	"github.com/bluegardenproject/tracks/internal/state"
)

// startService brings one declared service up for the track: it runs the
// pre_start hooks, launches the process in its own group, waits for the
// readiness probe, then runs the post_start hooks — persisting the
// service's status at each step. The command, env, hooks, and probe port
// are all templated (so {{.Port "name"}} resolves). worktree is the
// directory everything runs in (the service's repo worktree).
//
// It blocks until the service is ready (or the readiness timeout fires).
// On any failure the process group is torn down and the service is marked
// failed, so a half-started service never lingers.
func (s *Server) startService(ctx context.Context, sup *supervisor, t state.Track, svc config.Service, worktree string) (state.ServiceState, error) {
	data := services.NewTemplateData(t.ID, worktree, t.Ports)
	cmd, err := services.Render(svc.Cmd, data)
	if err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: render cmd: %w", svc.Name, err)
	}
	env, err := services.RenderEnv(svc.Env, data)
	if err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: render env: %w", svc.Name, err)
	}
	probePort, err := services.Render(svc.Ready.Port, data)
	if err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: render ready.port: %w", svc.Name, err)
	}
	logPath, err := s.serviceLogPath(t.ID, svc.Name)
	if err != nil {
		return state.ServiceState{}, err
	}

	// pre_start runs before the process exists, so it can't reach the
	// service log used for hook output yet — but the log file is fine to
	// append to. A failing pre_start aborts the start entirely.
	if err := services.RunHooks(ctx, svc.PreStart, data, worktree, logPath); err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: pre_start: %w", svc.Name, err)
	}

	proc, err := services.Start(services.Spec{
		Name:    svc.Name,
		Cmd:     cmd,
		Env:     env,
		Dir:     worktree,
		LogPath: logPath,
	})
	if err != nil {
		return state.ServiceState{}, err
	}

	sup.svcMu.Lock()
	if sup.services == nil {
		sup.services = make(map[string]*services.Process)
	}
	sup.services[svc.Name] = proc
	sup.svcMu.Unlock()

	probe := services.Probe{Port: probePort, LogRegex: svc.Ready.LogRegex}
	now := time.Now().UTC()
	st := state.ServiceState{
		Name:      svc.Name,
		Status:    state.ServiceStarting,
		PID:       proc.PID,
		PGID:      proc.PGID,
		Port:      t.Ports[svc.Name],
		LogPath:   logPath,
		StartedAt: &now,
	}
	if probe.IsZero() {
		// No probe: we can't assert it's serving, only that it launched.
		st.Status = state.ServiceRunning
	}
	if err := s.persistService(t.ID, t, st); err != nil {
		s.failService(sup, svc.Name)
		return state.ServiceState{}, err
	}

	// Wait for readiness, then run post_start. Any failure tears the
	// process down and records the service as failed.
	if err := services.WaitReady(ctx, probe, logPath, services.DefaultReadyTimeout); err != nil {
		s.markServiceFailed(sup, t.ID, svc.Name)
		return state.ServiceState{}, fmt.Errorf("service %s: %w", svc.Name, err)
	}
	if err := services.RunHooks(ctx, svc.PostStart, data, worktree, logPath); err != nil {
		s.markServiceFailed(sup, t.ID, svc.Name)
		return state.ServiceState{}, fmt.Errorf("service %s: post_start: %w", svc.Name, err)
	}

	if !probe.IsZero() {
		st.Status = state.ServiceReady
		if err := s.persistService(t.ID, t, st); err != nil {
			s.failService(sup, svc.Name)
			return state.ServiceState{}, err
		}
	}
	return st, nil
}

// persistService upserts a ServiceState onto the track via an atomic
// store update, so it can't clobber concurrent field updates from the
// supervisor poll loop (we only own the Services field). fallback is used
// when the track has somehow gone from the store.
func (s *Server) persistService(trackID string, fallback state.Track, st state.ServiceState) error {
	_, found, err := s.store.Update(trackID, func(t *state.Track) bool {
		t.Services = upsertService(t.Services, st)
		return true
	})
	if err != nil {
		return fmt.Errorf("persist service state: %w", err)
	}
	if !found {
		fallback.Services = upsertService(fallback.Services, st)
		if err := s.store.Put(fallback); err != nil {
			return fmt.Errorf("persist service state: %w", err)
		}
	}
	return nil
}

// failService tears down a service's process group and drops its handle,
// without touching persisted state (used when persistence itself failed).
func (s *Server) failService(sup *supervisor, name string) {
	sup.svcMu.Lock()
	proc := sup.services[name]
	delete(sup.services, name)
	sup.svcMu.Unlock()
	if proc != nil {
		proc.Stop(0)
	}
}

// markServiceFailed tears the process down and records the service as
// failed on the track, via an atomic update so it doesn't clobber the
// poll loop's concurrent writes.
func (s *Server) markServiceFailed(sup *supervisor, trackID, name string) {
	s.failService(sup, name)
	now := time.Now().UTC()
	_, _, _ = s.store.Update(trackID, func(t *state.Track) bool {
		for i := range t.Services {
			if t.Services[i].Name == name {
				t.Services[i].Status = state.ServiceFailed
				t.Services[i].ExitedAt = &now
				return true
			}
		}
		return false
	})
}

// serviceLogPath is where a service's stdout+stderr are streamed, under
// <state_dir>/logs/services/<track-id>-<service>.log.
func (s *Server) serviceLogPath(trackID, name string) (string, error) {
	dir, err := s.config().ResolveStateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "logs", "services", trackID+"-"+name+".log"), nil
}

// upsertService replaces the entry with the same name, or appends it.
func upsertService(list []state.ServiceState, st state.ServiceState) []state.ServiceState {
	for i := range list {
		if list[i].Name == st.Name {
			list[i] = st
			return list
		}
	}
	return append(list, st)
}

// stopPersistedServices tears down every still-running service recorded
// on the track by signalling its process group — the authoritative,
// state-driven teardown that works without a live in-memory handle (e.g.
// after Claude exited, or across a daemon restart). force skips the
// SIGTERM grace and SIGKILLs straight away. Returns an updated slice
// with the stopped entries marked.
func stopPersistedServices(svcs []state.ServiceState, force bool) []state.ServiceState {
	if len(svcs) == 0 {
		return svcs
	}
	now := time.Now().UTC()
	out := make([]state.ServiceState, len(svcs))
	copy(out, svcs)
	for i := range out {
		if !out[i].Status.Live() || out[i].PGID <= 0 {
			continue
		}
		if force {
			killPGID(out[i].PGID)
		} else {
			terminatePGID(out[i].PGID, 5*time.Second)
		}
		out[i].Status = state.ServiceStopped
		out[i].ExitedAt = &now
	}
	return out
}
