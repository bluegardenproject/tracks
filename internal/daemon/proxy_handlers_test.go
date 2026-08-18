package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/proxy"
	"github.com/bluegardenproject/tracks/internal/state"
)

// proxyTestServer is a readiness test server with a live proxy manager and
// one track running a "dev" service on the given port.
func proxyTestServer(t *testing.T, trackID string, devPort int) *Server {
	t.Helper()
	srv := newReadinessTestServer(t)
	mgr := proxy.NewManager()
	srv.mu.Lock()
	srv.proxyMgr = mgr
	srv.mu.Unlock()
	if err := srv.store.Put(state.Track{
		ID:       trackID,
		Branch:   "fix/x",
		Status:   state.StatusRunning,
		Ports:    map[string]int{"dev": devPort},
		Services: []state.ServiceState{{Name: "dev", Status: state.ServiceReady, Port: devPort}},
	}); err != nil {
		t.Fatal(err)
	}
	return srv
}

func TestProxyAddSwitchPersistsUpstream(t *testing.T) {
	const trackID = "20260101-000000-aaaaaa"
	srv := proxyTestServer(t, trackID, 24010)

	if resp := srv.handleProxyAdd(mustParams(t, ProxyAddParams{PublicPort: 3000})); !resp.Ok {
		t.Fatalf("add: %s", resp.Error)
	}
	// A switch to a service of a different-from-port name proves ports are
	// not tied to a service name.
	if resp := srv.handleProxySwitch(mustParams(t, ProxySwitchParams{PublicPort: 3000, TrackID: trackID, Service: "dev"})); !resp.Ok {
		t.Fatalf("switch: %s", resp.Error)
	}

	// State persisted the upstream.
	got := srv.store.AllProxies()
	if len(got) != 1 || got[0].UpstreamTrackID != trackID || got[0].UpstreamService != "dev" {
		t.Fatalf("binding not persisted with upstream: %+v", got)
	}

	// Status reports it active, naming the track and service.
	var res ProxyStatusResult
	resp := srv.handleProxyStatus()
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Proxies) != 1 || res.Proxies[0].ActiveService != "dev" || res.Proxies[0].ActiveTrackID != trackID {
		t.Fatalf("status = %+v, want :3000 active → %s/dev", res.Proxies, trackID)
	}

	// Clearing frees it but keeps the port defined.
	if resp := srv.handleProxySwitch(mustParams(t, ProxySwitchParams{PublicPort: 3000, TrackID: "off"})); !resp.Ok {
		t.Fatalf("clear: %s", resp.Error)
	}
	if got := srv.store.AllProxies(); len(got) != 1 || got[0].UpstreamTrackID != "" {
		t.Fatalf("clear did not reset the upstream: %+v", got)
	}
}

func TestProxySwitchRejectsUndefinedPort(t *testing.T) {
	srv := proxyTestServer(t, "20260101-000000-aaaaaa", 24010)
	resp := srv.handleProxySwitch(mustParams(t, ProxySwitchParams{PublicPort: 3000, TrackID: "20260101-000000-aaaaaa", Service: "dev"}))
	if resp.Ok {
		t.Fatal("switch on an undefined port should fail")
	}
}

// Ending or killing a track must free every port pointing at it and forget
// the persisted upstream — the worktree is gone, so nothing re-applies it.
func TestReleaseTrackProxies(t *testing.T) {
	const trackID = "20260101-000000-aaaaaa"
	srv := proxyTestServer(t, trackID, 24010)
	_ = srv.handleProxyAdd(mustParams(t, ProxyAddParams{PublicPort: 3000}))
	if resp := srv.handleProxySwitch(mustParams(t, ProxySwitchParams{PublicPort: 3000, TrackID: trackID, Service: "dev"})); !resp.Ok {
		t.Fatalf("switch: %s", resp.Error)
	}

	srv.releaseTrackProxies(trackID)

	if e := srv.proxyManager().Entry(3000); e != nil && e.Upstream() != "" {
		t.Errorf(":3000 still bound after the track ended: %q", e.Upstream())
	}
	b, ok := srv.store.GetProxy(3000)
	if !ok {
		t.Fatal(":3000 binding was deleted; it should only be freed")
	}
	if b.UpstreamTrackID != "" {
		t.Errorf(":3000 kept a dead upstream %q after the track ended", b.UpstreamTrackID)
	}
}

func TestProxyRemoveDeletesBinding(t *testing.T) {
	srv := proxyTestServer(t, "20260101-000000-aaaaaa", 24010)
	_ = srv.handleProxyAdd(mustParams(t, ProxyAddParams{PublicPort: 3000}))
	if resp := srv.handleProxyRemove(mustParams(t, ProxyRemoveParams{PublicPort: 3000})); !resp.Ok {
		t.Fatalf("remove: %s", resp.Error)
	}
	if len(srv.store.AllProxies()) != 0 {
		t.Error("binding survived remove")
	}
}

// reapplyProxyUpstreams binds a persisted upstream only when the target
// service is still running, and resets it to free otherwise.
func TestReapplyProxyUpstreams(t *testing.T) {
	const trackID = "20260101-000000-aaaaaa"
	srv := proxyTestServer(t, trackID, 24010)

	// Live target: re-apply binds it. syncProxies registers the port from
	// state first, exactly as Start does before reapply.
	if err := srv.store.PutProxy(state.ProxyBinding{PublicPort: 3000, UpstreamTrackID: trackID, UpstreamService: "dev"}); err != nil {
		t.Fatal(err)
	}
	srv.syncProxies()
	srv.reapplyProxyUpstreams()
	if e := srv.proxyManager().Entry(3000); e == nil || e.Upstream() == "" {
		t.Fatalf(":3000 not re-applied for a live target: %+v", e)
	}

	// Stopped target: re-apply frees it and clears the persisted upstream.
	srv.store.Update(trackID, func(tr *state.Track) bool {
		tr.Services[0].Status = state.ServiceStopped
		return true
	})
	if err := srv.store.PutProxy(state.ProxyBinding{PublicPort: 8081, UpstreamTrackID: trackID, UpstreamService: "dev"}); err != nil {
		t.Fatal(err)
	}
	srv.syncProxies()
	srv.reapplyProxyUpstreams()
	if e := srv.proxyManager().Entry(8081); e != nil && e.Upstream() != "" {
		t.Errorf(":8081 bound to a stopped target: %q", e.Upstream())
	}
	for _, b := range srv.store.AllProxies() {
		if b.PublicPort == 8081 && b.UpstreamTrackID != "" {
			t.Error(":8081 kept a dead upstream after re-apply")
		}
	}
}

// seedLegacyProxyPorts imports config proxy_port declarations once, when
// state has no ports of its own.
func TestSeedLegacyProxyPorts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	cfgYAML := "" +
		"repos:\n" +
		"  - name: buy-sell\n" +
		"    path: /tmp/bs\n" +
		"    base: main\n" +
		"    services:\n" +
		"      - name: dev\n" +
		"        cmd: pnpm dev\n" +
		"        proxy_port: 3000\n" +
		"      - name: metro\n" +
		"        cmd: pnpm start\n" +
		"        proxy_port: 8081\n" +
		"        proxy_bind_all: true\n"
	writeConfig(t, dir, cfgYAML)

	srv := newReadinessTestServer(t)
	srv.mu.Lock()
	srv.proxyMgr = proxy.NewManager()
	srv.mu.Unlock()

	srv.seedLegacyProxyPorts()

	got := srv.store.AllProxies()
	if len(got) != 2 {
		t.Fatalf("seeded %d ports, want 2: %+v", len(got), got)
	}
	if got[0].PublicPort != 3000 || got[1].PublicPort != 8081 || !got[1].BindAll {
		t.Fatalf("seeded bindings wrong: %+v", got)
	}

	// Idempotent: a second seed with ports already present is a no-op.
	srv.seedLegacyProxyPorts()
	if len(srv.store.AllProxies()) != 2 {
		t.Error("second seed added duplicates")
	}
}

// writeConfig writes config.yaml at <XDG_CONFIG_HOME>/tracks/config.yaml,
// the path config.Path() resolves to when XDG_CONFIG_HOME is set.
func writeConfig(t *testing.T, xdgDir, body string) {
	t.Helper()
	dir := filepath.Join(xdgDir, "tracks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
