package daemon

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
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
// progress live; readiness is the human's (or a later probe's) concern.
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

	paneCmd := buildServicePaneCommand(env, steps, serverCmd, logPath)
	panePID, err := s.openServerPane(sup, svc.Name, t.Ports[svc.Name], paneCmd, worktree)
	if err != nil {
		return state.ServiceState{}, fmt.Errorf("service %s: open pane: %w", svc.Name, err)
	}

	now := time.Now().UTC()
	st := state.ServiceState{
		Name:      svc.Name,
		Status:    state.ServiceRunning,
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
	return st, nil
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
