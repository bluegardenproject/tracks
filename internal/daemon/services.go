package daemon

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/services"
	"github.com/bluegardenproject/tracks/internal/state"
)

// startService launches one declared service for the track as a
// supervised background process and records it on the track. It renders
// the command and env templates (so {{.Port "name"}} resolves), starts
// the process in its own group, registers the in-memory handle on the
// supervisor, and persists a ServiceState. Readiness waiting and
// lifecycle hooks are layered on separately — this is the raw start plus
// state bookkeeping. worktree is the directory the command runs in (the
// service's repo worktree).
func (s *Server) startService(sup *supervisor, t state.Track, svc config.Service, worktree string) (state.ServiceState, error) {
	data := services.NewTemplateData(t.ID, worktree, t.Ports)
	cmd, err := services.Render(svc.Cmd, data)
	if err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: render cmd: %w", svc.Name, err)
	}
	env, err := services.RenderEnv(svc.Env, data)
	if err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: render env: %w", svc.Name, err)
	}
	logPath, err := s.serviceLogPath(t.ID, svc.Name)
	if err != nil {
		return state.ServiceState{}, err
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

	now := time.Now().UTC()
	st := state.ServiceState{
		Name:      svc.Name,
		Status:    state.ServiceRunning,
		PID:       proc.PID,
		PGID:      proc.PGID,
		Port:      t.Ports[svc.Name],
		LogPath:   logPath,
		StartedAt: &now,
	}

	// Persist on a fresh read so we don't clobber concurrent updates
	// from the supervisor's poll loop; we only own the Services field.
	cur, ok := s.store.Get(t.ID)
	if !ok {
		cur = t
	}
	cur.Services = upsertService(cur.Services, st)
	if err := s.store.Put(cur); err != nil {
		proc.Stop(0)
		sup.svcMu.Lock()
		delete(sup.services, svc.Name)
		sup.svcMu.Unlock()
		return state.ServiceState{}, fmt.Errorf("persist service state: %w", err)
	}
	return st, nil
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
		if out[i].Status != state.ServiceRunning || out[i].PGID <= 0 {
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
