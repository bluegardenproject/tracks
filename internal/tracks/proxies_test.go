package tracks

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/procs"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

func TestProxyPlan(t *testing.T) {
	f := newFixture(t)
	servers := withServers(t, f, "api", []store.DevServer{
		{Name: "web", Command: "pnpm dev", PortMode: store.PortAssigned},
		{Name: "sb", Command: "pnpm sb", PortMode: store.PortAssigned},
	})
	ctx := context.Background()
	tr := work(t, f, "api")
	if _, err := f.svc.Up(ctx, tr.ID, "web"); err != nil {
		t.Fatal(err)
	}
	window := "@" + tr.Name
	servers.mu.Lock()
	servers.panes[window][0].PID = 200
	servers.panes[window] = append(servers.panes[window], tmux.Pane{ID: "%t", Role: trackwin.RoleTerminal, PID: 300})
	servers.mu.Unlock()
	ps := map[int]procs.Proc{
		200: {PID: 200, PPID: 1, Command: "sh"}, 201: {PID: 201, PPID: 200, Command: "node vite"},
		300: {PID: 300, PPID: 1, Command: "bash"}, 301: {PID: 301, PPID: 300, Command: "python3 -m http.server"},
		400: {PID: 400, PPID: 1, Command: "/usr/local/bin/node other.js"},
	}
	ports := map[int][]int{201: {20000}, 301: {8000, 3004}, 400: {3003}}
	f.svc.Snapshot = func(context.Context) (procs.Snapshot, error) { return procs.New(ps, ports), nil }

	for _, port := range []int{3000, 3001, 3002, 3003, 3004, 3005} {
		if err := f.svc.AddProxyPort(ctx, port); err != nil {
			t.Fatal(err)
		}
	}
	inputs := map[int]store.ProxyInput{
		3000: {Track: tr.ID, Repo: "api", Server: "web"},
		3001: {Track: tr.ID, Port: 8000},
		3002: {Track: tr.ID, Repo: "api", Server: "sb"},
		3003: {Track: tr.ID, Repo: "api", Server: "web"},
		3004: {Track: tr.ID, Port: 3004},
	}
	for port, in := range inputs {
		if err := f.svc.SetProxyInput(ctx, port, in); err != nil {
			t.Fatal(err)
		}
	}

	view, want, err := f.svc.ProxyPlan(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Inputs) != 3 || InputOf(view.Inputs[0]) != inputs[3000] || InputOf(view.Inputs[2]) != inputs[3001] {
		t.Errorf("inputs = %+v; want web and the servers on 3004 and 8000", view.Inputs)
	}
	states := map[int]string{}
	for _, p := range view.Ports {
		states[p.Port] = p.State
	}
	if states[3000] != ProxyForwarding || states[3001] != ProxyForwarding || states[3002] != ProxyNoServer ||
		states[3003] != ProxyBlocked || states[3004] != ProxyBlocked || states[3005] != ProxyIdle {
		t.Errorf("states = %v; want 3003 held by another program and 3004 its own input, both blocked", states)
	}
	if want[3000] != 20000 || want[3001] != 8000 || want[3002] != 0 || len(want) != 3 {
		t.Errorf("want = %v; want 3000→20000, 3001→8000, 3002 answering 503, the rest not listened on", want)
	}
	if h := view.Ports[3].Holder; h != "node (pid 400)" {
		t.Errorf("3003's holder = %q; want node", h)
	}

	if err := f.svc.SetProxyInput(ctx, 3000, store.ProxyInput{}); err != nil {
		t.Fatal(err)
	}
	if _, want, _ = f.svc.ProxyPlan(ctx, false); len(want) != 2 {
		t.Errorf("after clearing 3000: want %v", want)
	}
	for _, bad := range []store.ProxyInput{{Track: tr.ID}, {Track: tr.ID, Server: "web"}, {Track: tr.ID, Repo: "api", Server: "web", Port: 1}} {
		if err := f.svc.SetProxyInput(ctx, 3000, bad); !isProblem(err) {
			t.Errorf("SetProxyInput(%+v) = %v; want a problem", bad, err)
		}
	}
	f.svc.Snapshot = func(context.Context) (procs.Snapshot, error) { return procs.Snapshot{}, errors.New("lsof failed") }
	if _, _, err := f.svc.ProxyPlan(ctx, false); err == nil {
		t.Error("ProxyPlan without the processes: want an error, not every port without its server")
	}
}

func TestAddProxyPortChecks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for port, part := range map[int]string{80: "1024 to 65535", 70000: "1024 to 65535", 20005: "dev servers"} {
		if err := f.svc.AddProxyPort(ctx, port); !isProblem(err) || !strings.Contains(err.Error(), part) {
			t.Errorf("AddProxyPort(%d) = %v; want a problem about %s", port, err, part)
		}
	}
	if err := f.svc.AddProxyPort(ctx, 3000); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.AddProxyPort(ctx, 3000); !isProblem(err) {
		t.Errorf("3000 twice: %v", err)
	}
	if err := f.svc.RemoveProxyPort(ctx, 4000); !isProblem(err) {
		t.Errorf("removing a port not listed: %v", err)
	}
}
