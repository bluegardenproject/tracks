package tracks

import (
	"context"
	"slices"
	"testing"

	"github.com/bluegardenproject/tracks/internal/procs"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

func TestServerStatesFindWhatListens(t *testing.T) {
	f := newFixture(t)
	servers := withServers(t, f, "api", []store.DevServer{
		{Name: "web", Command: "pnpm dev", PortMode: store.PortDetect},
		{Name: "sb", Command: "pnpm sb", PortMode: store.PortAssigned, Type: "storybook"},
	})
	ctx := context.Background()
	tr := work(t, f, "api")
	if _, err := f.svc.Up(ctx, tr.ID, ""); err != nil {
		t.Fatal(err)
	}
	window := "@" + tr.Name
	servers.mu.Lock()
	pids := map[string]int{"api/web": 200, "api/sb": 210}
	for i, p := range servers.panes[window] {
		servers.panes[window][i].PID = pids[p.Key]
	}
	servers.panes[window] = append(servers.panes[window],
		tmux.Pane{ID: "%agent", Role: trackwin.RoleAgent, PID: 100},
		tmux.Pane{ID: "%term", Role: trackwin.RoleTerminal, PID: 300},
		// An agent pane whose agent exited runs a login shell.
		tmux.Pane{ID: "%done", Role: trackwin.RoleAgent, PID: 400})
	servers.mu.Unlock()

	ps := map[int]procs.Proc{
		100: {PID: 100, PPID: 1, Command: "sh -c claude"},
		101: {PID: 101, PPID: 100, Command: "claude go"},
		102: {PID: 102, PPID: 101, Command: "node mcp-server.js"},
		103: {PID: 103, PPID: 101, Command: "/bin/zsh -c pnpm dev"},
		104: {PID: 104, PPID: 103, Command: "node /src/node_modules/.bin/vite"},
		105: {PID: 105, PPID: 101, Command: "npm exec @acme/browser-mcp"},
		106: {PID: 106, PPID: 105, Command: "sh -c browser-mcp"},
		107: {PID: 107, PPID: 106, Command: "node browser-mcp.js"},
		211: {PID: 211, PPID: 210, Command: "node --inspect storybook.js"},
		400: {PID: 400, PPID: 1, Command: "-zsh"},
		401: {PID: 401, PPID: 400, Command: "python3 -m http.server 9000"},
		200: {PID: 200, PPID: 1, Command: "sh -c pnpm dev"},
		201: {PID: 201, PPID: 200, Command: "node /src/node_modules/@rspack/cli/bin/rspack.js serve"},
		210: {PID: 210, PPID: 1, Command: "sh -c pnpm sb"},
		300: {PID: 300, PPID: 1, Command: "bash -l"},
		301: {PID: 301, PPID: 300, Command: "python3 -m http.server"},
	}
	ports := map[int][]int{101: {55000}, 102: {3845}, 104: {5173}, 107: {3999}, 201: {4000}, 211: {9229}, 301: {8000}, 401: {9000}}
	f.svc.Snapshot = func(context.Context) (procs.Snapshot, error) { return procs.New(ps, ports), nil }

	got, err := f.svc.ServerStates(ctx, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	base := Server{Track: tr.ID, TrackName: tr.Name}
	want := []Server{
		with(base, func(s *Server) {
			s.Repo, s.Name, s.Mode, s.Port, s.State, s.Type, s.Inferred = "api", "web", store.PortDetect, 4000, ServerReady, "rspack", true
		}),
		with(base, func(s *Server) {
			s.Repo, s.Name, s.Mode, s.Port, s.State, s.Type = "api", "sb", store.PortAssigned, 20000, ServerStarting, "storybook"
		}),
		with(base, func(s *Server) {
			s.Mode, s.Port, s.State, s.Type, s.Inferred = store.PortDetect, 5173, ServerReady, "vite", true
		}),
		with(base, func(s *Server) {
			s.Mode, s.Port, s.State, s.Type, s.Inferred = store.PortDetect, 8000, ServerReady, "python3", true
		}),
		with(base, func(s *Server) {
			s.Mode, s.Port, s.State, s.Type, s.Inferred = store.PortDetect, 9000, ServerReady, "python3", true
		}),
	}
	if !slices.Equal(got, want) {
		t.Errorf("ServerStates =\n%+v\nwant\n%+v\n(the agent's own port and its MCP servers' don't count; sb's debugger port doesn't make it ready)", got, want)
	}
	all, err := f.svc.AllServers(ctx)
	if err != nil || !slices.Equal(all, want) {
		t.Errorf("AllServers = %+v, %v; want the one track's", all, err)
	}
}

func with(s Server, set func(*Server)) Server {
	set(&s)
	return s
}
