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

func TestRegisterDoesNotBind(t *testing.T) {
	port := freePort(t)
	m := NewManager()
	m.Register(Registration{ServiceName: "metro", PublicPort: port})
	if !canBind(port) {
		t.Fatalf("port %d was bound by Register; expected it to stay free until Switch", port)
	}
}

func TestSwitchBindsAndClearReleases(t *testing.T) {
	proxyPort := freePort(t)
	upstream := freePort(t)
	m := NewManager()
	m.Register(Registration{ServiceName: "metro", PublicPort: proxyPort})

	if err := m.Switch("metro", upstream); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if canBind(proxyPort) {
		t.Fatalf("port %d still free after Switch; expected the proxy to hold it", proxyPort)
	}
	if got := m.Entry("metro").Upstream(); got != fmt.Sprintf("localhost:%d", upstream) {
		t.Fatalf("upstream = %q, want localhost:%d", got, upstream)
	}

	m.Clear("metro")
	if !canBind(proxyPort) {
		t.Fatalf("port %d still held after Clear; expected it released for a manual dev server", proxyPort)
	}
	if got := m.Entry("metro").Upstream(); got != "" {
		t.Fatalf("upstream = %q after Clear, want empty", got)
	}
}

func TestSwitchRebindsAfterClear(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	m.Register(Registration{ServiceName: "metro", PublicPort: proxyPort})

	for i := 0; i < 3; i++ {
		if err := m.Switch("metro", freePort(t)); err != nil {
			t.Fatalf("Switch #%d: %v", i, err)
		}
		m.Clear("metro")
		if !canBind(proxyPort) {
			t.Fatalf("port %d not released after Clear #%d", proxyPort, i)
		}
	}
}

func TestSwitchUnknownService(t *testing.T) {
	m := NewManager()
	if err := m.Switch("nope", 1234); err == nil {
		t.Fatal("Switch on unregistered service: want error, got nil")
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
	m.Register(Registration{ServiceName: "metro", PublicPort: port})
	if err := m.Switch("metro", freePort(t)); err == nil {
		t.Fatal("Switch onto an occupied port: want bind error, got nil")
	}
}

func TestStopReleasesBoundPorts(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	m.Register(Registration{ServiceName: "metro", PublicPort: proxyPort})
	if err := m.Switch("metro", freePort(t)); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	m.Stop()
	if !canBind(proxyPort) {
		t.Fatalf("port %d still held after Stop", proxyPort)
	}
}

func TestListenAddrDefaultsToLoopback(t *testing.T) {
	e := &Entry{ServiceName: "metro", PublicPort: 8081}
	if got := e.listenAddr(); got != "127.0.0.1:8081" {
		t.Errorf("listenAddr = %q, want 127.0.0.1:8081 — a dev server must not be offered to the whole network by default", got)
	}
	e.BindAll = true
	if got := e.listenAddr(); got != ":8081" {
		t.Errorf("listenAddr with BindAll = %q, want :8081", got)
	}
}

// The loopback default must still serve the machine's own browser.
func TestSwitchServesOverLoopback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("from upstream"))
	}))
	defer upstream.Close()
	upstreamPort := mustPort(t, upstream.URL)

	proxyPort := freePort(t)
	m := NewManager()
	m.Register(Registration{ServiceName: "metro", PublicPort: proxyPort})
	if err := m.Switch("metro", upstreamPort); err != nil {
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

func TestSyncRegistersNewServices(t *testing.T) {
	m := NewManager()
	m.Sync([]Registration{{ServiceName: "metro", PublicPort: 8081}})
	if m.Entry("metro") == nil {
		t.Fatal("metro not registered by Sync")
	}
	// A service added to the config later shows up without a restart.
	m.Sync([]Registration{
		{ServiceName: "metro", PublicPort: 8081},
		{ServiceName: "api", PublicPort: 9000},
	})
	if m.Entry("api") == nil {
		t.Error("api added by a later Sync was not registered")
	}
}

func TestSyncKeepsAServingEntryUntouched(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	reg := Registration{ServiceName: "metro", PublicPort: proxyPort}
	m.Register(reg)
	if err := m.Switch("metro", freePort(t)); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	defer m.Stop()
	want := m.Entry("metro").Upstream()

	m.Sync([]Registration{reg})

	if got := m.Entry("metro").Upstream(); got != want {
		t.Errorf("upstream = %q after Sync, want %q — a config reload must not interrupt a serving track", got, want)
	}
	if canBind(proxyPort) {
		t.Error("proxy released its port during an unrelated config reload")
	}
}

func TestSyncReleasesRemovedServices(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	m.Register(Registration{ServiceName: "metro", PublicPort: proxyPort})
	if err := m.Switch("metro", freePort(t)); err != nil {
		t.Fatalf("Switch: %v", err)
	}

	m.Sync(nil) // service deleted from the config

	if m.Entry("metro") != nil {
		t.Error("metro still registered after it left the config")
	}
	if !canBind(proxyPort) {
		t.Errorf("port %d still held after the service left the config", proxyPort)
	}
}

func TestSyncRebuildsOnPortChange(t *testing.T) {
	oldPort := freePort(t)
	newPort := freePort(t)
	m := NewManager()
	m.Register(Registration{ServiceName: "metro", PublicPort: oldPort})
	if err := m.Switch("metro", freePort(t)); err != nil {
		t.Fatalf("Switch: %v", err)
	}

	m.Sync([]Registration{{ServiceName: "metro", PublicPort: newPort}})
	defer m.Stop()

	if got := m.Entry("metro").PublicPort; got != newPort {
		t.Errorf("PublicPort = %d, want %d", got, newPort)
	}
	if !canBind(oldPort) {
		t.Errorf("old port %d still held after the config moved the service", oldPort)
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

// A port change on a *serving* service must not leave the track behind a
// dead stable port: the upstream moves to the new listener.
func TestSyncCarriesTheUpstreamAcrossARebuild(t *testing.T) {
	oldPort := freePort(t)
	newPort := freePort(t)
	upstream := freePort(t)
	m := NewManager()
	m.Register(Registration{ServiceName: "metro", PublicPort: oldPort})
	if err := m.Switch("metro", upstream); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	defer m.Stop()

	m.Sync([]Registration{{ServiceName: "metro", PublicPort: newPort}})

	e := m.Entry("metro")
	if got, want := e.Upstream(), fmt.Sprintf("localhost:%d", upstream); got != want {
		t.Errorf("upstream = %q after the port moved, want %q", got, want)
	}
	if canBind(newPort) {
		t.Errorf("new port %d not bound — the service is serving but unreachable through the proxy", newPort)
	}
	if !canBind(oldPort) {
		t.Errorf("old port %d still held", oldPort)
	}
}

// Flipping proxy_bind_all mid-session is the same rebuild path.
func TestSyncCarriesTheUpstreamAcrossABindChange(t *testing.T) {
	port := freePort(t)
	upstream := freePort(t)
	m := NewManager()
	m.Register(Registration{ServiceName: "metro", PublicPort: port})
	if err := m.Switch("metro", upstream); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	defer m.Stop()

	m.Sync([]Registration{{ServiceName: "metro", PublicPort: port, BindAll: true}})

	e := m.Entry("metro")
	if !e.BindAll {
		t.Error("BindAll not applied")
	}
	if got, want := e.Upstream(), fmt.Sprintf("localhost:%d", upstream); got != want {
		t.Errorf("upstream = %q, want %q — flipping the switch dropped a live upstream", got, want)
	}
}

// A config reload racing `tracks up` must not bind a port on an entry
// that is no longer registered — nothing could ever release it. Run under
// -race; the leak shows up as a port still held after Stop.
func TestSyncRacingSwitchDoesNotLeakAListener(t *testing.T) {
	ports := make([]int, 8)
	for i := range ports {
		ports[i] = freePort(t)
	}
	m := NewManager()
	m.Register(Registration{ServiceName: "metro", PublicPort: ports[0]})

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
			_ = m.Switch("metro", up)
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 40 {
			m.Sync([]Registration{{ServiceName: "metro", PublicPort: ports[i%len(ports)]}})
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
