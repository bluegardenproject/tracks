package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/notify"
	"github.com/bluegardenproject/tracks/internal/services"
	"github.com/bluegardenproject/tracks/internal/state"
)

// handleServiceUp starts a named service (and its declared depends_on
// dependencies) for a track. Each service runs in its own tmux pane in the
// track window and *owns* its process (see startServicePane); the call returns
// as soon as the panes are opened — dependency install and the server come up
// live in the pane, not behind a blocking wait. The stable-port proxy (if the
// service declares proxy_port) is pointed at this track immediately.
func (s *Server) handleServiceUp(ctx context.Context, raw json.RawMessage, emit Emit) Response {
	var p ServiceUpParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}

	t, found := s.store.Get(p.TrackID)
	if !found {
		return fail("track not found: " + p.TrackID)
	}
	if t.Status.IsTerminal() {
		return fail("track is no longer active")
	}

	s.mu.Lock()
	sup := s.supervisors[p.TrackID]
	s.mu.Unlock()
	if sup == nil {
		return fail("no running supervisor for track " + p.TrackID + " (is it a work track that is still running?)")
	}

	cfg := s.config()

	// Build a map of all services available across this track's repos, so we
	// can resolve depends_on and find worktree paths + the deferred deps cmd.
	type svcEntry struct {
		svc      config.Service
		worktree string
		depsCmd  string
	}
	allSvcs := make(map[string]svcEntry)
	deps := make(map[string][]string)

	for _, trackRepo := range t.Repos {
		cfgRepo, ok := cfg.RepoByName(trackRepo.Name)
		if !ok {
			continue
		}
		depsCmd := ""
		if cfgRepo.Provision != nil {
			depsCmd = cfgRepo.Provision.DepsCmd
		}
		for _, svc := range cfgRepo.Services {
			allSvcs[svc.Name] = svcEntry{svc: svc, worktree: trackRepo.Path, depsCmd: depsCmd}
			deps[svc.Name] = svc.DependsOn
		}
	}

	if _, ok := allSvcs[p.ServiceName]; !ok {
		return fail(fmt.Sprintf("service %q not found in any repo of this track", p.ServiceName))
	}

	// Resolve full start order including dependencies.
	order, err := services.StartOrder([]string{p.ServiceName}, deps)
	if err != nil {
		return fail("resolve service order: " + err.Error())
	}

	// Open each service's pane in dependency order, skipping those already live.
	for _, name := range order {
		fresh, ok := s.store.Get(p.TrackID)
		if !ok {
			return fail("track disappeared mid-start")
		}
		if serviceLive(fresh.Services, name) {
			if name == p.ServiceName {
				emit(name + ": already running")
			} else {
				emit("dependency " + name + ": already running, skipping")
			}
			continue
		}

		entry := allSvcs[name]
		if name == p.ServiceName {
			emit("starting " + name + "…")
		} else {
			emit("starting dependency " + name + "…")
		}

		st, err := s.startServicePane(sup, fresh, entry.svc, entry.worktree, entry.depsCmd)
		if err != nil {
			return fail(fmt.Sprintf("start %s: %v", name, err))
		}
		emit(fmt.Sprintf("%s launched on :%d (installing deps + starting in its pane)", name, st.Port))
	}

	port := t.Ports[p.ServiceName]
	logPath, _ := s.serviceLogPath(t.ID, p.ServiceName)

	// Point the stable-port proxy at this track's service now. The upstream
	// 503s until the server actually binds the port, then self-heals — we no
	// longer wait for readiness before switching.
	s.mu.Lock()
	mgr := s.proxyMgr
	s.mu.Unlock()
	proxyPort := 0
	if mgr != nil {
		if entry := mgr.Entry(p.ServiceName); entry != nil {
			if err := mgr.Switch(p.ServiceName, port); err == nil {
				proxyPort = entry.PublicPort
				emit(fmt.Sprintf("proxy :%d → localhost:%d", proxyPort, port))
			}
		}
	}

	body := fmt.Sprintf("%s — http://localhost:%d", p.ServiceName, port)
	if proxyPort > 0 {
		body = fmt.Sprintf("%s — stable: http://localhost:%d  track: http://localhost:%d",
			p.ServiceName, proxyPort, port)
	}
	s.notifyEvent(string(notify.EventServiceReady), "tracks: dev server started", body)

	return ok(ServiceUpResult{Port: port, LogPath: logPath})
}

// handleServiceDown stops a single running service, running its pre_stop
// hooks first, killing its process group, and closing its pane.
func (s *Server) handleServiceDown(ctx context.Context, raw json.RawMessage, emit Emit) Response {
	var p ServiceDownParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}

	t, found := s.store.Get(p.TrackID)
	if !found {
		return fail("track not found: " + p.TrackID)
	}

	var svcState *state.ServiceState
	for i := range t.Services {
		if t.Services[i].Name == p.ServiceName {
			ss := t.Services[i]
			svcState = &ss
			break
		}
	}
	if svcState == nil || !svcState.Status.Live() {
		return fail(fmt.Sprintf("service %q is not running", p.ServiceName))
	}

	cfg := s.config()

	// Run pre_stop hooks before terminating.
	for _, trackRepo := range t.Repos {
		cfgRepo, ok := cfg.RepoByName(trackRepo.Name)
		if !ok {
			continue
		}
		for _, svc := range cfgRepo.Services {
			if svc.Name == p.ServiceName && len(svc.PreStop) > 0 {
				data := services.NewTemplateData(t.ID, trackRepo.Path, t.Ports)
				emit("running pre_stop hooks for " + p.ServiceName)
				if err := services.RunHooks(ctx, svc.PreStop, data, trackRepo.Path, svcState.LogPath); err != nil {
					// pre_stop failures are logged but don't abort teardown.
					emit("pre_stop warning: " + err.Error())
				}
			}
		}
	}

	emit("stopping " + p.ServiceName + "…")

	// The pane owns the process; the persisted PGID (= pane pid, the group
	// leader) is the authoritative teardown handle.
	if svcState.PGID > 0 {
		terminatePGID(svcState.PGID, 5*time.Second)
	}

	now := time.Now().UTC()
	_, _, _ = s.store.Update(p.TrackID, func(t *state.Track) bool {
		for i := range t.Services {
			if t.Services[i].Name == p.ServiceName {
				t.Services[i].Status = state.ServiceStopped
				t.Services[i].ExitedAt = &now
				return true
			}
		}
		return false
	})

	s.mu.Lock()
	sup := s.supervisors[p.TrackID]
	s.mu.Unlock()
	if sup != nil {
		s.closeServerPane(sup, p.ServiceName)
	}

	// Clear the stable-port proxy upstream for this service.
	s.mu.Lock()
	mgr := s.proxyMgr
	s.mu.Unlock()
	if mgr != nil {
		mgr.Clear(p.ServiceName)
	}

	emit(p.ServiceName + " stopped")
	return ok(nil)
}

// handleServices returns the current service states and port map for a track.
func (s *Server) handleServices(raw json.RawMessage) Response {
	var p ServicesParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return fail("bad params: " + err.Error())
	}

	t, found := s.store.Get(p.TrackID)
	if !found {
		return fail("track not found: " + p.TrackID)
	}

	return ok(ServicesResult{
		Services: t.Services,
		Ports:    t.Ports,
	})
}

// serviceLive reports whether the named service is in a live state in svcs.
func serviceLive(svcs []state.ServiceState, name string) bool {
	for _, ss := range svcs {
		if ss.Name == name && ss.Status.Live() {
			return true
		}
	}
	return false
}
