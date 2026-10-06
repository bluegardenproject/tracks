package tracks

import (
	"context"
	"errors"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/bluegardenproject/tracks/internal/procs"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// fakeServers keeps dev-server panes per window; a test sets their
// state.
type fakeServers struct {
	mu      sync.Mutex
	panes   map[string][]tmux.Pane
	added   []string // keys
	stopped []string // keys
	n       int
}

func (f *fakeServers) Panes(window string) ([]tmux.Pane, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.panes[window]), nil
}

func (f *fakeServers) AddDevServer(window, dir string, p trackwin.Process, key string, port int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.panes == nil {
		f.panes = map[string][]tmux.Pane{}
	}
	f.n++
	f.added = append(f.added, key)
	f.panes[window] = append(f.panes[window], tmux.Pane{ID: "%" + string(rune('a'+f.n)), Role: trackwin.RoleDevServer,
		Title: p.Title, Key: key, Port: port, State: ServerRunning})
	return nil
}

func (f *fakeServers) StopPane(p tmux.Pane) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stopped = append(f.stopped, p.Key)
	for w, panes := range f.panes {
		f.panes[w] = slices.DeleteFunc(panes, func(q tmux.Pane) bool { return q.ID == p.ID })
	}
	return nil
}

func (f *fakeServers) Capture(pane string, lines int) (string, error) {
	return "output of " + pane, nil
}

func (f *fakeServers) setState(key, state string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, panes := range f.panes {
		for i := range panes {
			if panes[i].Key == key {
				panes[i].State = state
			}
		}
	}
}

// withServers gives the fixture's repo name the dev servers defs and
// returns fake panes for them. busy are ports something else listens on.
func withServers(t *testing.T, f *fixture, name string, defs []store.DevServer, busy ...int) *fakeServers {
	t.Helper()
	withServersAlso(t, f, name, defs)
	servers := &fakeServers{}
	f.worktrees.root = t.TempDir()
	f.svc.Servers = servers
	f.svc.CopyEnv = func(context.Context, string, string) ([]string, error) { return nil, nil }
	f.svc.PortFree = func(port int) bool { return !slices.Contains(busy, port) }
	f.svc.Snapshot = func(context.Context) (procs.Snapshot, error) { return procs.New(nil, nil), nil }
	return servers
}

func work(t *testing.T, f *fixture, repos ...string) track.Track {
	t.Helper()
	got, err := f.svc.Create(context.Background(), Request{Kind: track.Work, Repos: repos, Prompt: "go"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	return got.Track
}

func TestUpAndDown(t *testing.T) {
	f := newFixture(t)
	servers := withServers(t, f, "api", []store.DevServer{
		{Name: "web", Command: "pnpm dev", PortMode: store.PortAssigned, Type: "rspack"},
		{Name: "storybook", Command: "pnpm sb", PortMode: store.PortAssigned},
		{Name: "metro", Command: "pnpm start", PortMode: store.PortFixed, Port: 8081},
		{Name: "app", Command: "pnpm app", PortMode: store.PortDetect},
	}, 20000)
	ctx := context.Background()
	tr := work(t, f, "api")

	got, err := f.svc.Up(ctx, tr.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []Server{
		{Track: tr.ID, TrackName: tr.Name, Repo: "api", Name: "web", Type: "rspack", Mode: store.PortAssigned, Port: 20001, State: ServerStarting},
		{Track: tr.ID, TrackName: tr.Name, Repo: "api", Name: "storybook", Mode: store.PortAssigned, Port: 20002, State: ServerStarting},
		{Track: tr.ID, TrackName: tr.Name, Repo: "api", Name: "metro", Mode: store.PortFixed, Port: 8081, State: ServerStarting},
		{Track: tr.ID, TrackName: tr.Name, Repo: "api", Name: "app", Mode: store.PortDetect, State: ServerStarting},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Up = %+v\nwant %+v (20000 is busy)", got, want)
	}
	if _, err := f.svc.Up(ctx, tr.ID, ""); err != nil || len(servers.added) != 4 {
		t.Fatalf("Up again: %v, %d panes; want the running ones left alone", err, len(servers.added))
	}

	servers.setState("api/web", "exited 1")
	got, _ = f.svc.ServerStates(ctx, tr.ID)
	if got[0].State != ServerCrashed || got[0].Code != 1 {
		t.Errorf("web after exit 1: %+v; want crashed", got[0])
	}
	if got, _ = f.svc.Up(ctx, tr.ID, "web"); got[0].State != ServerStarting || got[0].Port != 20001 ||
		!slices.Equal(servers.stopped, []string{"api/web"}) {
		t.Errorf("Up web after its crash: %+v, stopped %v; want its old pane closed and it running on 20001", got[0], servers.stopped)
	}

	if text, err := f.svc.Logs(ctx, tr.ID, "storybook", 50); err != nil || !strings.HasPrefix(text, "output of ") {
		t.Errorf("Logs = %q, %v", text, err)
	}
	if got, _ = f.svc.Down(ctx, tr.ID, "api/storybook"); got[1].State != ServerStopped || got[0].State != ServerStarting {
		t.Errorf("Down storybook: %+v; want only it stopped", got)
	}
	if _, err := f.svc.Logs(ctx, tr.ID, "storybook", 50); !isProblem(err) {
		t.Errorf("Logs of a stopped server: %v; want a problem", err)
	}
	if got, _ = f.svc.Down(ctx, tr.ID, ""); slices.ContainsFunc(got, func(sv Server) bool { return sv.State != ServerStopped }) {
		t.Errorf("Down all: %+v", got)
	}
}

func TestUpChecksNamesAndPorts(t *testing.T) {
	f := newFixture(t)
	withServers(t, f, "api", []store.DevServer{{Name: "metro", Command: "a", PortMode: store.PortFixed, Port: 8081}})
	servers := withServers(t, f, "web", []store.DevServer{{Name: "metro", Command: "b", PortMode: store.PortFixed, Port: 8081}})
	ctx := context.Background()
	first := work(t, f, "api")
	second := work(t, f, "api", "web")

	if _, err := f.svc.Up(ctx, second.ID, "metro"); !isProblem(err) || !strings.Contains(err.Error(), "repo/server") {
		t.Errorf("an ambiguous name: %v; want a problem asking for repo/server", err)
	}
	if _, err := f.svc.Up(ctx, second.ID, "nope"); !isProblem(err) || !strings.Contains(err.Error(), "api/metro, web/metro") {
		t.Errorf("an unknown name: %v; want the servers listed", err)
	}
	if _, err := f.svc.Up(ctx, first.ID, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := f.svc.Up(ctx, second.ID, "api/metro"); err != nil || !strings.Contains(got[0].Problem, "api/metro in "+first.Name) {
		t.Errorf("a fixed port another track holds: %+v, %v; want that track named", got, err)
	}
	f.svc.PortFree = func(int) bool { return false }
	if _, err := f.svc.Up(ctx, first.ID, ""); err != nil {
		t.Errorf("Up on a running server needs no port: %v", err)
	}
	if len(servers.added) != 1 {
		t.Errorf("%d panes; want only the first track's metro", len(servers.added))
	}
}

func isProblem(err error) bool {
	var p Problem
	return errors.As(err, &p)
}

func TestUpStartsWhatItCan(t *testing.T) {
	f := newFixture(t)
	servers := withServers(t, f, "api", []store.DevServer{
		{Name: "a", Command: "x", PortMode: store.PortFixed, Port: 8081},
		{Name: "b", Command: "y", PortMode: store.PortFixed, Port: 8082},
		{Name: "c", Command: "z", Dir: "apps/missing", PortMode: store.PortAssigned},
		{Name: "d", Command: "w", PortMode: store.PortAssigned},
	}, 8082)
	withServersAlso(t, f, "web", []store.DevServer{{Name: "e", Command: "v", PortMode: store.PortFixed, Port: 8081}})
	ctx := context.Background()
	tr := work(t, f, "api", "web")
	got, err := f.svc.Up(ctx, tr.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	problems := map[string]string{}
	for _, sv := range got {
		problems[sv.Name] = sv.Problem
	}
	if problems["a"] != "" || problems["d"] != "" || len(servers.added) != 2 {
		t.Errorf("problems %q, %d started; want a and d started", problems, len(servers.added))
	}
	for name, want := range map[string]string{"b": "another program", "c": "isn't on this branch", "e": "another server of this track"} {
		if !strings.Contains(problems[name], want) {
			t.Errorf("%s: %q; want it to say %q", name, problems[name], want)
		}
	}
}

// withServersAlso gives repo name dev servers too, keeping the fakes.
func withServersAlso(t *testing.T, f *fixture, name string, defs []store.DevServer) {
	t.Helper()
	ctx := context.Background()
	repos, err := f.store.Repos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range repos {
		if r.Name == name {
			r.Servers = defs
			if _, err := f.store.UpdateRepo(ctx, r); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestPortFreeSeesLoopbackListeners(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1"} {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
		if err != nil {
			t.Logf("no %s here: %v", host, err)
			continue
		}
		port := ln.Addr().(*net.TCPAddr).Port
		if portFree(port) {
			t.Errorf("port %d with a listener on %s counts as free", port, host)
		}
		_ = ln.Close()
		if !portFree(port) {
			t.Errorf("port %d counts as taken after its listener closed", port)
		}
	}
}

func TestThePromptKnowsTheDevServers(t *testing.T) {
	f := newFixture(t)
	withServers(t, f, "api", []store.DevServer{{Name: "web", Command: "pnpm dev", PortMode: store.PortAssigned}})
	work(t, f, "web")
	if f.engine.spec.DevServers {
		t.Error("a track whose repos have no dev servers was told about them")
	}
	work(t, f, "api")
	if !f.engine.spec.DevServers {
		t.Error("a track whose repo has dev servers wasn't told about them")
	}

	ctx := context.Background()
	plan, err := f.svc.Create(ctx, Request{Kind: track.Plan, Repos: []string{"api"}, Prompt: "Plan it"}, func(string) {})
	if err != nil || f.engine.spec.DevServers {
		t.Fatalf("a Plan track, without worktrees: %v, told about dev servers %v", err, f.engine.spec.DevServers)
	}
	if _, err := f.svc.Promote(ctx, plan.Track.ID, func(string) {}); err != nil || !f.engine.spec.DevServers {
		t.Errorf("promoted to Work: %v, told about dev servers %v; want told", err, f.engine.spec.DevServers)
	}
}
