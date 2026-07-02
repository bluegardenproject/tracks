package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/notify"
	"github.com/bluegardenproject/tracks/internal/services"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

// handleServiceUp starts a named service (and its declared depends_on
// dependencies) for a track, waits for readiness, opens log-viewer panes,
// and fires a service_ready notification.
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

	// Build a map of all services available across this track's repos,
	// so we can resolve depends_on and find worktree paths.
	type svcEntry struct {
		svc      config.Service
		worktree string
	}
	allSvcs := make(map[string]svcEntry)
	deps := make(map[string][]string)

	for _, trackRepo := range t.Repos {
		cfgRepo, ok := cfg.RepoByName(trackRepo.Name)
		if !ok {
			continue
		}
		for _, svc := range cfgRepo.Services {
			allSvcs[svc.Name] = svcEntry{
				svc:      svc,
				worktree: trackRepo.Path,
			}
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

	// Start each service in dependency order, skipping those already live.
	for _, name := range order {
		// Re-fetch for fresh service state after each iteration.
		fresh, ok := s.store.Get(p.TrackID)
		if !ok {
			return fail("track disappeared mid-start")
		}
		alreadyLive := false
		for _, ss := range fresh.Services {
			if ss.Name == name && ss.Status.Live() {
				alreadyLive = true
				break
			}
		}
		if alreadyLive {
			if name == p.ServiceName {
				emit(name + ": already running")
			} else {
				emit("dependency " + name + ": already running, skipping")
			}
			continue
		}

		entry, ok := allSvcs[name]
		if !ok {
			return fail(fmt.Sprintf("dependency %q not found in track repos", name))
		}

		if name == p.ServiceName {
			emit("starting " + name + "…")
		} else {
			emit("starting dependency " + name + "…")
		}

		st, err := s.startService(ctx, sup, fresh, entry.svc, entry.worktree)
		if err != nil {
			return fail(fmt.Sprintf("start %s: %v", name, err))
		}

		emit(fmt.Sprintf("%s ready on :%d", name, st.Port))
		s.openViewerPane(cfg.Tmux.SessionName, sup, name, st.Port, st.LogPath)
	}

	port := t.Ports[p.ServiceName]
	logPath, _ := s.serviceLogPath(t.ID, p.ServiceName)

	// Auto-switch the stable-port proxy to this track's service if configured.
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
	s.notifyEvent(string(notify.EventServiceReady), "tracks: service ready", body)

	return ok(ServiceUpResult{Port: port, LogPath: logPath})
}

// handleServiceDown stops a single running service, running its pre_stop
// hooks first, and closes the log-viewer pane.
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

	s.mu.Lock()
	sup := s.supervisors[p.TrackID]
	s.mu.Unlock()

	if sup != nil {
		sup.svcMu.Lock()
		proc := sup.services[p.ServiceName]
		delete(sup.services, p.ServiceName)
		sup.svcMu.Unlock()
		if proc != nil {
			proc.Stop(5 * time.Second)
		} else if svcState.PGID > 0 {
			terminatePGID(svcState.PGID, 5*time.Second)
		}
	} else if svcState.PGID > 0 {
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

	if sup != nil {
		s.closeViewerPane(sup, p.ServiceName)
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

// openViewerPane opens a tail-f pane for the service's log file in the right
// column of the track's tmux window. The first service splits the window right
// (35%); each subsequent service stacks below the previous one. Viewer pane
// failures are logged but never fatal — the service process already runs.
func (s *Server) openViewerPane(session string, sup *supervisor, svcName string, port int, logPath string) {
	tm := tmux.New()
	title := fmt.Sprintf("%s:%d", svcName, port)

	// tail -n 100: show the last 100 lines first so there's context when
	// the pane opens, then follow new output.
	quotedPath := "'" + strings.ReplaceAll(logPath, "'", "'\\''") + "'"
	cmd := "tail -n 100 -f " + quotedPath

	sup.svcMu.Lock()
	defer sup.svcMu.Unlock()

	if sup.viewerPanes == nil {
		sup.viewerPanes = make(map[string]string)
	}

	var paneID string
	var err error

	if len(sup.viewerPanes) == 0 {
		// First service — split the track window horizontally.
		paneID, err = tm.SplitWindowRight(session, sup.windowName, cmd, 35)
	} else {
		// Subsequent services — stack below the last right-column pane.
		var lastPane string
		for _, id := range sup.viewerPanes {
			lastPane = id
		}
		paneID, err = tm.SplitPaneDown(lastPane, cmd)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "tracks: open viewer pane for %s: %v\n", svcName, err)
		return
	}

	_ = tm.SetPaneTitle(paneID, title)
	sup.viewerPanes[svcName] = paneID
}

// closeViewerPane kills the log-viewer pane for the named service.
func (s *Server) closeViewerPane(sup *supervisor, svcName string) {
	sup.svcMu.Lock()
	defer sup.svcMu.Unlock()
	if sup.viewerPanes == nil {
		return
	}
	paneID, ok := sup.viewerPanes[svcName]
	if !ok {
		return
	}
	delete(sup.viewerPanes, svcName)
	_ = tmux.New().KillPane(paneID)
}
