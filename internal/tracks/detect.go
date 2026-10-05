package tracks

import (
	"context"
	"errors"
	"strings"

	"github.com/bluegardenproject/tracks/internal/procs"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// agentPrograms are the agent CLIs' programs, which an agent pane runs.
var agentPrograms = []string{"claude", "agent", "cursor-agent"}

// isAgent reports whether command runs an agent CLI, an npm-installed
// Claude Code included.
func isAgent(command string) bool {
	for _, p := range agentPrograms {
		if procs.Runs(command, p) {
			return true
		}
	}
	return strings.Contains(command, "/@anthropic-ai/claude-code/")
}

// ServerStates is the state of each of track id's dev servers, then the
// servers found listening in its other panes.
func (s *Service) ServerStates(ctx context.Context, id string) ([]Server, error) {
	return s.statesOf(ctx, id, s.snapshot(ctx))
}

func (s *Service) statesOf(ctx context.Context, id string, snap procs.Snapshot) ([]Server, error) {
	t, window, all, err := s.serversOf(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.serverStates(t, window, all, snap, true)
}

// statesIn is statesOf for track id in window, which the caller found.
func (s *Service) statesIn(ctx context.Context, id, window string, snap procs.Snapshot) ([]Server, error) {
	if s.Servers == nil {
		return nil, errors.New("dev servers need tmux")
	}
	t, err := s.Store.Track(ctx, id)
	if err != nil {
		return nil, err
	}
	all, err := s.devServers(ctx, t)
	if err != nil {
		return nil, err
	}
	return s.serverStates(t, window, all, snap, true)
}

// AllServers is ServerStates of every open track with a window.
func (s *Service) AllServers(ctx context.Context) ([]Server, error) {
	return s.allServers(ctx, s.snapshot(ctx))
}

func (s *Service) allServers(ctx context.Context, snap procs.Snapshot) ([]Server, error) {
	infos, err := s.Windows.List()
	if err != nil {
		return nil, err
	}
	var out []Server
	for _, in := range infos {
		if in.Track == "" {
			continue
		}
		servers, err := s.statesIn(ctx, in.Track, in.Window, snap)
		if errors.Is(err, store.ErrNotFound) {
			continue // a window whose track is being created or is gone
		} else if err != nil {
			return nil, err
		}
		out = append(out, servers...)
	}
	return out, nil
}

func (s *Service) serverStates(t track.Track, window string, all []devServer, snap procs.Snapshot, detected bool) ([]Server, error) {
	panes, err := s.Servers.Panes(window)
	if err != nil {
		return nil, err
	}
	out := make([]Server, 0, len(all))
	for _, d := range all {
		sv := Server{Track: t.ID, TrackName: t.Name, Repo: d.repo.Name, Name: d.def.Name, Type: d.def.Type,
			Mode: d.def.PortMode, State: ServerStopped}
		if p, ok := serverPane(panes, d.key()); ok {
			sv.State, sv.Port, sv.Code = serverState(p), p.Port, exitCode(p.State)
			if sv.State == ServerRunning {
				sv = listening(sv, snap, snap.Below(p.PID))
			}
		}
		out = append(out, sv)
	}
	if detected {
		out = append(out, found(t, panes, snap)...)
	}
	return out, nil
}

// listening sets running sv ready when a process in its pane listens on
// its port, or for a detect-mode server, on the first port found.
func listening(sv Server, snap procs.Snapshot, ls []procs.Listener) Server {
	if len(ls) == 0 {
		sv.State = ServerStarting
		return sv
	}
	// A server with its own port is ready once that port listens: a
	// debugger or HMR socket in the same pane doesn't count.
	at, ok := ls[0], sv.Mode == store.PortDetect
	for _, l := range ls {
		if l.Port == sv.Port {
			at, ok = l, true
		}
	}
	if !ok {
		sv.State = ServerStarting
		return sv
	}
	sv.State, sv.Port = ServerReady, at.Port
	if sv.Type == "" {
		sv.Type, sv.Inferred = procs.Kind(snap.Procs[at.PID].Command), true
	}
	return sv
}

// found are the servers listening in t's panes that aren't its dev
// servers: what an agent or a terminal started.
func found(t track.Track, panes []tmux.Pane, snap procs.Snapshot) []Server {
	var out []Server
	seen := map[int]bool{}
	for _, p := range panes {
		if p.Role == trackwin.RoleDevServer {
			continue
		}
		agent := 0
		if p.Role == trackwin.RoleAgent {
			agent = agentOf(snap, p.PID)
		}
		for _, l := range snap.Below(p.PID) {
			if l.Depth < 1 || seen[l.Port] || (agent != 0 && !throughShell(snap, l.PID, agent)) {
				continue
			}
			seen[l.Port] = true
			out = append(out, Server{Track: t.ID, TrackName: t.Name, Mode: store.PortDetect, Port: l.Port,
				State: ServerReady, Type: procs.Kind(snap.Procs[l.PID].Command), Inferred: true})
		}
	}
	return out
}

// agentOf is the agent process below pane process pid, nearest first;
// 0 once the agent exited and the pane runs its shell.
func agentOf(snap procs.Snapshot, pid int) int {
	queue := []int{pid}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		if isAgent(snap.Procs[p].Command) {
			return p
		}
		queue = append(queue, snap.Children(p)...)
	}
	return 0
}

// throughShell reports whether pid, a process below the agent, runs
// through a shell the agent started: a command the agent ran. The
// agent itself and what it starts directly, such as its MCP servers,
// don't. A process not below the agent counts too.
func throughShell(snap procs.Snapshot, pid, agent int) bool {
	for cur, steps := pid, 0; cur != 0 && steps < 64; cur, steps = snap.Parent(cur), steps+1 {
		if cur == agent {
			return false
		}
		if snap.Parent(cur) == agent {
			return procs.IsShell(snap.Procs[cur].Command)
		}
	}
	return true
}

// snapshot reads the processes and their ports. Without them no server
// counts as listening, which only delays ready.
func (s *Service) snapshot(ctx context.Context) procs.Snapshot {
	snap, err := s.takeSnapshot(ctx)
	if err != nil {
		return procs.New(nil, nil)
	}
	return snap
}

func (s *Service) takeSnapshot(ctx context.Context) (procs.Snapshot, error) {
	if s.Snapshot != nil {
		return s.Snapshot(ctx)
	}
	return procs.Take(ctx)
}
