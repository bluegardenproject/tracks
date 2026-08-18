package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/dlog"
	"github.com/bluegardenproject/tracks/internal/notify"
	"github.com/bluegardenproject/tracks/internal/services"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

// startServicePane brings one declared service up for the track by opening a
// dedicated tmux pane in the track window and running its start steps there:
// the (optional) dependency-install command, any pre_start hooks, then the
// server command — all templated and joined with `&&`, teed to the service
// log so `tracks services` and `tail` can read the output.
//
// The pane *owns* the process: tmux setsid's each pane, so the pane's pid is
// the process-group leader. We record it as the service's PGID, which is the
// authoritative teardown handle (endTrack/recovery/daemon-shutdown all kill
// the group). We do NOT block on readiness — a slow `pnpm install` used to
// overrun the caller's timeout and make the start look broken. The pane shows
// progress live, and a service that declares a `ready:` probe has it resolved
// in the background by watchServiceReady.
func (s *Server) startServicePane(sup *supervisor, t state.Track, svc config.Service, worktree, depsCmd string) (state.ServiceState, error) {
	data := services.NewTemplateData(t.ID, worktree, t.Ports)
	serverCmd, err := services.Render(svc.Cmd, data)
	if err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: render cmd: %w", svc.Name, err)
	}
	env, err := services.RenderEnv(svc.Env, data)
	if err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: render env: %w", svc.Name, err)
	}

	// Steps that must succeed before the server starts, in order: deps
	// install (deferred from worktree creation) then any pre_start hooks.
	var steps []string
	if strings.TrimSpace(depsCmd) != "" {
		rendered, err := services.Render(depsCmd, data)
		if err != nil {
			return state.ServiceState{}, fmt.Errorf("service %s: render deps_cmd: %w", svc.Name, err)
		}
		steps = append(steps, rendered)
	}
	for i, hook := range svc.PreStart {
		rendered, err := services.Render(hook, data)
		if err != nil {
			return state.ServiceState{}, fmt.Errorf("service %s: render pre_start[%d]: %w", svc.Name, i, err)
		}
		steps = append(steps, rendered)
	}

	logPath, err := s.serviceLogPath(t.ID, svc.Name)
	if err != nil {
		return state.ServiceState{}, err
	}

	probe, err := renderProbe(svc.Ready, data)
	if err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: %w", svc.Name, err)
	}

	paneCmd := buildServicePaneCommand(env, steps, serverCmd, logPath)
	panePID, err := s.openServerPane(sup, svc.Name, t.Ports[svc.Name], paneCmd, worktree)
	if err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: open pane: %w", svc.Name, err)
	}

	// A service with a readiness probe or post_start hooks isn't usable
	// yet — it's "starting" until watchServiceReady says otherwise. One
	// with neither has nothing left to observe, so it goes straight to
	// running (we can't assert it's serving, only that it launched).
	status := state.ServiceRunning
	if !probe.IsZero() || len(svc.PostStart) > 0 {
		status = state.ServiceStarting
	}

	now := time.Now().UTC()
	st := state.ServiceState{
		Name:      svc.Name,
		Status:    status,
		PID:       panePID,
		PGID:      panePID,
		Port:      t.Ports[svc.Name],
		LogPath:   logPath,
		StartedAt: &now,
	}
	if err := s.persistService(t.ID, t, st); err != nil {
		// The pane is already running; tear it down so state and reality
		// don't diverge, then surface the error.
		terminatePGID(panePID, 0)
		s.closeServerPane(sup, svc.Name)
		return state.ServiceState{}, err
	}
	if status == state.ServiceStarting {
		go s.watchServiceReady(sup, t.ID, svc, probe, worktree, logPath, t.Ports, panePID)
	}
	return st, nil
}

// renderProbe resolves a service's configured readiness probe against the
// track's template data, so `port: '{{.Port "live-app"}}'` becomes the
// port actually allocated to this track.
func renderProbe(p config.ReadyProbe, data services.TemplateData) (services.Probe, error) {
	if p.IsZero() {
		return services.Probe{}, nil
	}
	port, err := services.Render(p.Port, data)
	if err != nil {
		return services.Probe{}, fmt.Errorf("render ready.port: %w", err)
	}
	return services.Probe{Port: strings.TrimSpace(port), LogRegex: p.LogRegex}, nil
}

// watchServiceReady waits for a freshly-started service to become usable,
// then runs its post_start hooks and marks it ready — the transition the
// `service_ready` notification announces.
//
// It runs in its own goroutine so `tracks up` stays non-blocking: a slow
// `pnpm install` in the pane used to overrun the caller's timeout and make
// a perfectly healthy start look broken. The probe is what makes the
// difference between "the pane opened" and "the server is answering", so
// nothing here is on the request path.
func (s *Server) watchServiceReady(sup *supervisor, trackID string, svc config.Service, probe services.Probe, worktree, logPath string, ports map[string]int, pgid int) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Give up the moment the track ends or the daemon shuts down. In that
	// case the service's end state belongs to teardown, not to us — see
	// the ctx.Err() checks below.
	go func() {
		select {
		case <-sup.done:
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := services.WaitReady(ctx, probe, logPath, s.probeTimeout()); err != nil {
		if ctx.Err() != nil {
			return // torn down while waiting; teardown owns the status
		}
		s.failService(trackID, svc.Name, pgid, "never became ready", err, logPath)
		return
	}
	if len(svc.PostStart) > 0 {
		data := services.NewTemplateData(trackID, worktree, ports)
		if err := services.RunHooks(ctx, svc.PostStart, data, worktree, logPath); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.failService(trackID, svc.Name, pgid, "post_start hooks failed", err, logPath)
			return
		}
	}
	if !s.markService(trackID, svc.Name, pgid, state.ServiceReady) {
		return // stopped, replaced, or gone while we waited — nothing to announce
	}
	s.notifyEvent(string(notify.EventServiceReady), "tracks: dev server ready",
		svc.Name+" — "+s.serviceURLs(trackID, svc.Name, ports[svc.Name]))
}

// probeTimeout is how long a readiness probe may take before the service
// is called failed.
func (s *Server) probeTimeout() time.Duration {
	if s.readyTimeout > 0 {
		return s.readyTimeout
	}
	return services.DefaultReadyTimeout
}

// failService records a service as failed and tells the user where to
// look. The pane is deliberately left alone: a server that missed the
// readiness deadline may still be coming up, and killing it would throw
// away the log the user needs. It stays in NeedsTeardown, so the track's
// teardown still reclaims the process and its port.
func (s *Server) failService(trackID, name string, pgid int, what string, cause error, logPath string) {
	dlog.Printf("service %s on track %s: %s: %v", name, trackID, what, cause)
	if !s.markService(trackID, name, pgid, state.ServiceFailed) {
		return
	}
	s.notifyEvent(string(notify.EventServiceFailed), "tracks: dev server problem",
		fmt.Sprintf("%s %s — see %s", name, what, logPath))
}

// markService moves one of a track's services to a new status, reporting
// whether the change was applied.
//
// pgid identifies the instance the caller is speaking for. A service is
// only a name in the state file, but `tracks down web` followed by
// `tracks up web` inside the probe window produces two watchers for that
// one name — and the first one, still polling the port its dead instance
// never opened, would otherwise mark the *new* instance failed (or re-run
// its post_start hooks). Matching on the process group makes a watcher
// unable to speak for an instance it didn't start. Pass 0 to skip the
// check when there is no instance to tie the change to.
//
// A service that is no longer live (stopped by the user, or torn down
// with the track) is likewise left alone: a probe that finishes
// afterwards must not resurrect it as ready.
func (s *Server) markService(trackID, name string, pgid int, status state.ServiceStatus) bool {
	applied := false
	s.update(trackID, "service status", func(t *state.Track) bool {
		for i := range t.Services {
			if t.Services[i].Name != name {
				continue
			}
			if pgid != 0 && t.Services[i].PGID != pgid {
				return false // a later instance owns this name now
			}
			if !t.Services[i].Status.Live() || t.Services[i].Status == status {
				return false
			}
			t.Services[i].Status = status
			if status == state.ServiceFailed {
				now := time.Now().UTC()
				t.Services[i].ExitedAt = &now
			}
			applied = true
			return true
		}
		return false
	})
	return applied
}

// serviceURLs renders the address(es) a ready service is reachable at:
// the track's own port, plus the stable proxy port when one forwards to it.
func (s *Server) serviceURLs(trackID, name string, port int) string {
	if mgr := s.proxyManager(); mgr != nil {
		if publicPort, ok := mgr.ActivePortFor(trackID, name); ok {
			return fmt.Sprintf("stable: http://localhost:%d  track: http://localhost:%d", publicPort, port)
		}
	}
	return fmt.Sprintf("http://localhost:%d", port)
}

// buildServicePaneCommand assembles the single shell command a service pane
// runs, wrapped in a login shell (`$SHELL -lc`) so PATH carries the node/pnpm
// that nvm/fnm put there, matching how Claude itself is spawned.
func buildServicePaneCommand(env map[string]string, steps []string, serverCmd, logPath string) string {
	return "exec ${SHELL:-/bin/bash} -lc " + shellQuoteSvc(buildServiceScript(env, steps, serverCmd, logPath))
}

// buildServiceScript is the un-wrapped shell script buildServicePaneCommand
// runs inside the login shell: env exports, then the ordered steps + server
// command (short-circuited with `&&`) teed to the log, then a fallback
// interactive shell so the pane never dies to a blank "[exited]" and the
// worktree stays pokeable. Split out from the wrapper so it can be asserted on
// without the outer shell-quoting.
func buildServiceScript(env map[string]string, steps []string, serverCmd, logPath string) string {
	var b strings.Builder
	for _, k := range sortedKeys(env) {
		b.WriteString("export " + k + "=" + shellQuoteSvc(env[k]) + "; ")
	}
	seq := make([]string, 0, len(steps)+1)
	for _, s := range steps {
		if strings.TrimSpace(s) != "" {
			seq = append(seq, s)
		}
	}
	if strings.TrimSpace(serverCmd) != "" {
		seq = append(seq, serverCmd)
	}
	b.WriteString("{ " + strings.Join(seq, " && ") + " ; } 2>&1 | tee " + shellQuoteSvc(logPath) + "; ")
	b.WriteString("exec ${SHELL:-/bin/bash} -l")
	return b.String()
}

// openServerPane opens the service's pane in the right column of the track
// window and returns the pid of the pane's process (the group leader). The
// first service splits the window right (30%); each subsequent service stacks
// below the previous one. The pane runs from worktree so relative paths in the
// command resolve there.
func (s *Server) openServerPane(sup *supervisor, svcName string, port int, command, worktree string) (panePID int, err error) {
	tm := tmux.New()
	session := s.config().Tmux.SessionName

	sup.svcMu.Lock()
	defer sup.svcMu.Unlock()
	if sup.servicePanes == nil {
		sup.servicePanes = make(map[string]string)
	}

	var paneID string
	if len(sup.servicePanes) == 0 {
		paneID, panePID, err = tm.SplitWindowRight(session, sup.windowName, command, worktree, 30)
	} else {
		paneID, panePID, err = tm.SplitPaneDown(sup.lastServicePane, command, worktree)
	}
	if err != nil {
		return 0, err
	}
	_ = tm.SetPaneTitle(paneID, fmt.Sprintf("%s:%d", svcName, port))
	sup.servicePanes[svcName] = paneID
	sup.lastServicePane = paneID
	return panePID, nil
}

// closeServerPane kills the pane for the named service (cosmetic — the
// authoritative teardown is the process-group kill by PGID).
func (s *Server) closeServerPane(sup *supervisor, svcName string) {
	sup.svcMu.Lock()
	defer sup.svcMu.Unlock()
	if sup.servicePanes == nil {
		return
	}
	paneID, ok := sup.servicePanes[svcName]
	if !ok {
		return
	}
	delete(sup.servicePanes, svcName)
	_ = tmux.New().KillPane(paneID)
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
		if !out[i].NeedsTeardown() {
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

// sortedKeys returns the map keys in deterministic order so a rendered
// command is reproducible (and testable).
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// shellQuoteSvc wraps s in single quotes with embedded single quotes
// escaped — safe to embed anywhere in a /bin/sh command line.
func shellQuoteSvc(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
