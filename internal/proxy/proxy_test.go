package proxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
)

// freePort asks the OS for an unused TCP port, then releases it so a
// caller can bind it. Racy in theory, fine for a single-process test.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// canBind reports whether the port is currently free to bind on
// loopback — the address the proxy claims (see Entry.listenAddr). macOS
// happily binds 0.0.0.0:P alongside a 127.0.0.1:P listener, so checking
// the wildcard address here would report a held port as free.
func canBind(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// switchTo is a test shorthand for Switch with placeholder labels.
func switchTo(m *Manager, port, upstream int) error {
	return m.Switch(port, upstream, "trk", "svc")
}

func TestRegisterDoesNotBind(t *testing.T) {
	port := freePort(t)
	m := NewManager()
	m.Register(Registration{PublicPort: port})
	if !canBind(port) {
		t.Fatalf("port %d was bound by Register; expected it to stay free until Switch", port)
	}
}

func TestSwitchBindsAndClearReleases(t *testing.T) {
	proxyPort := freePort(t)
	upstream := freePort(t)
	m := NewManager()
	m.Register(Registration{PublicPort: proxyPort})

	if err := switchTo(m, proxyPort, upstream); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if canBind(proxyPort) {
		t.Fatalf("port %d still free after Switch; expected the proxy to hold it", proxyPort)
	}
	if got := m.Entry(proxyPort).Upstream(); got != fmt.Sprintf("localhost:%d", upstream) {
		t.Fatalf("upstream = %q, want localhost:%d", got, upstream)
	}
	if tID, svc := m.Entry(proxyPort).UpstreamTarget(); tID != "trk" || svc != "svc" {
		t.Fatalf("upstream target = %q/%q, want trk/svc", tID, svc)
	}

	m.Clear(proxyPort)
	if !canBind(proxyPort) {
		t.Fatalf("port %d still held after Clear; expected it released for a manual dev server", proxyPort)
	}
	if got := m.Entry(proxyPort).Upstream(); got != "" {
		t.Fatalf("upstream = %q after Clear, want empty", got)
	}
}

func TestSwitchRebindsAfterClear(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	m.Register(Registration{PublicPort: proxyPort})

	for i := 0; i < 3; i++ {
		if err := switchTo(m, proxyPort, freePort(t)); err != nil {
			t.Fatalf("Switch #%d: %v", i, err)
		}
		m.Clear(proxyPort)
		if !canBind(proxyPort) {
			t.Fatalf("port %d not released after Clear #%d", proxyPort, i)
		}
	}
}

func TestSwitchUnknownPort(t *testing.T) {
	m := NewManager()
	if err := switchTo(m, 65000, 1234); err == nil {
		t.Fatal("Switch on unregistered port: want error, got nil")
	}
}

func TestSwitchBindFailureSurfaces(t *testing.T) {
	port := freePort(t)
	// Occupy the proxy port with a foreign listener, as a manual dev
	// server would.
	blocker, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer blocker.Close()

	m := NewManager()
	m.Register(Registration{PublicPort: port})
	if err := switchTo(m, port, freePort(t)); err == nil {
		t.Fatal("Switch onto an occupied port: want bind error, got nil")
	}
}

func TestRemoveReleasesPort(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	m.Register(Registration{PublicPort: proxyPort})
	if err := switchTo(m, proxyPort, freePort(t)); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	m.Remove(proxyPort)
	if !canBind(proxyPort) {
		t.Fatalf("port %d still held after Remove", proxyPort)
	}
	if m.Entry(proxyPort) != nil {
		t.Fatal("entry still registered after Remove")
	}
}

func TestStopReleasesBoundPorts(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	m.Register(Registration{PublicPort: proxyPort})
	if err := switchTo(m, proxyPort, freePort(t)); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	m.Stop()
	if !canBind(proxyPort) {
		t.Fatalf("port %d still held after Stop", proxyPort)
	}
}

func TestListenAddrDefaultsToLoopback(t *testing.T) {
	e := &Entry{PublicPort: 8081}
	if got := e.listenAddr(); got != "127.0.0.1:8081" {
		t.Errorf("listenAddr = %q, want 127.0.0.1:8081 — a dev server must not be offered to the whole network by default", got)
	}
	e.BindAll = true
	if got := e.listenAddr(); got != ":8081" {
		t.Errorf("listenAddr with BindAll = %q, want :8081", got)
	}
}

// ActivePortFor finds the public port forwarding to a given track/service.
func TestActivePortFor(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	m.Register(Registration{PublicPort: proxyPort})
	if err := m.Switch(proxyPort, freePort(t), "trkA", "dev"); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	defer m.Stop()
	if got, ok := m.ActivePortFor("trkA", "dev"); !ok || got != proxyPort {
		t.Fatalf("ActivePortFor(trkA, dev) = %d,%v; want %d,true", got, ok, proxyPort)
	}
	if _, ok := m.ActivePortFor("trkB", "dev"); ok {
		t.Fatal("ActivePortFor(trkB, dev) matched a different track")
	}
}

// The loopback default must still serve the machine's own browser, and a
// port can forward to a service of any name (cross-service targeting).
func TestSwitchServesOverLoopback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("from upstream"))
	}))
	defer upstream.Close()
	upstreamPort := mustPort(t, upstream.URL)

	proxyPort := freePort(t)
	m := NewManager()
	m.Register(Registration{PublicPort: proxyPort})
	// A port named for one thing (3000) fronting a service called
	// "swap-dev" — the binding is not tied to the service name.
	if err := m.Switch(proxyPort, upstreamPort, "trk", "swap-dev"); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	defer m.Stop()

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", proxyPort))
	if err != nil {
		t.Fatalf("GET through proxy: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "from upstream" {
		t.Errorf("body = %q, want %q", body, "from upstream")
	}
}

func TestSyncRegistersNewPorts(t *testing.T) {
	m := NewManager()
	m.Sync([]Registration{{PublicPort: 8081}})
	if m.Entry(8081) == nil {
		t.Fatal("8081 not registered by Sync")
	}
	// A port added later shows up without a restart.
	m.Sync([]Registration{
		{PublicPort: 8081},
		{PublicPort: 9000},
	})
	if m.Entry(9000) == nil {
		t.Error("9000 added by a later Sync was not registered")
	}
}

func TestSyncKeepsAServingEntryUntouched(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	reg := Registration{PublicPort: proxyPort}
	m.Register(reg)
	if err := switchTo(m, proxyPort, freePort(t)); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	defer m.Stop()
	want := m.Entry(proxyPort).Upstream()

	m.Sync([]Registration{reg})

	if got := m.Entry(proxyPort).Upstream(); got != want {
		t.Errorf("upstream = %q after Sync, want %q — a reconcile must not interrupt a serving port", got, want)
	}
	if canBind(proxyPort) {
		t.Error("proxy released its port during an unrelated reconcile")
	}
}

func TestSyncReleasesRemovedPorts(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	m.Register(Registration{PublicPort: proxyPort})
	if err := switchTo(m, proxyPort, freePort(t)); err != nil {
		t.Fatalf("Switch: %v", err)
	}

	m.Sync(nil) // port removed from state

	if m.Entry(proxyPort) != nil {
		t.Error("port still registered after it left state")
	}
	if !canBind(proxyPort) {
		t.Errorf("port %d still held after it left state", proxyPort)
	}
}

// mustPort extracts the port from an httptest server URL.
func mustPort(t *testing.T, rawURL string) int {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %s: %v", rawURL, err)
	}
	p, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatalf("port of %s: %v", rawURL, err)
	}
	return p
}

// Flipping BindAll mid-session is a rebuild path: a live upstream must
// survive it, else the serving track drops behind a dead stable port.
func TestSyncCarriesTheUpstreamAcrossABindChange(t *testing.T) {
	port := freePort(t)
	upstream := freePort(t)
	m := NewManager()
	m.Register(Registration{PublicPort: port})
	if err := m.Switch(port, upstream, "trk", "svc"); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	defer m.Stop()

	m.Sync([]Registration{{PublicPort: port, BindAll: true}})

	e := m.Entry(port)
	if !e.BindAll {
		t.Error("BindAll not applied")
	}
	if got, want := e.Upstream(), fmt.Sprintf("localhost:%d", upstream); got != want {
		t.Errorf("upstream = %q, want %q — flipping the switch dropped a live upstream", got, want)
	}
	if tID, svc := e.UpstreamTarget(); tID != "trk" || svc != "svc" {
		t.Errorf("upstream target = %q/%q after rebuild, want trk/svc", tID, svc)
	}
}

// A reconcile racing a Switch must not bind a port on an entry that is no
// longer registered — nothing could ever release it. Run under -race; the
// leak shows up as a port still held after Stop.
func TestSyncRacingSwitchDoesNotLeakAListener(t *testing.T) {
	ports := make([]int, 8)
	for i := range ports {
		ports[i] = freePort(t)
	}
	m := NewManager()
	m.Register(Registration{PublicPort: ports[0]})

	var wg sync.WaitGroup
	wg.Add(2)
	// Upstreams are reserved up front: freePort calls t.Fatalf, which does
	// not fail a test properly from a spawned goroutine.
	upstreams := make([]int, 40)
	for i := range upstreams {
		upstreams[i] = freePort(t)
	}
	go func() {
		defer wg.Done()
		for _, up := range upstreams {
			_ = m.Switch(ports[0], up, "trk", "svc")
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 40 {
			m.Sync([]Registration{{PublicPort: ports[i%len(ports)]}})
		}
	}()
	wg.Wait()

	m.Stop()
	for _, p := range ports {
		if !canBind(p) {
			t.Errorf("port %d still held after Stop — an orphaned listener leaked", p)
		}
	}
}
