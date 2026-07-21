package proxy

import (
	"fmt"
	"net"
	"testing"
)

// freePort asks the OS for an unused TCP port, then releases it so a
// caller can bind it. Racy in theory, fine for a single-process test.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// canBind reports whether the port is currently free to bind.
func canBind(port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

func TestRegisterDoesNotBind(t *testing.T) {
	port := freePort(t)
	m := NewManager()
	m.Register("metro", port)
	if !canBind(port) {
		t.Fatalf("port %d was bound by Register; expected it to stay free until Switch", port)
	}
}

func TestSwitchBindsAndClearReleases(t *testing.T) {
	proxyPort := freePort(t)
	upstream := freePort(t)
	m := NewManager()
	m.Register("metro", proxyPort)

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
	m.Register("metro", proxyPort)

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
	blocker, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer blocker.Close()

	m := NewManager()
	m.Register("metro", port)
	if err := m.Switch("metro", freePort(t)); err == nil {
		t.Fatal("Switch onto an occupied port: want bind error, got nil")
	}
}

func TestStopReleasesBoundPorts(t *testing.T) {
	proxyPort := freePort(t)
	m := NewManager()
	m.Register("metro", proxyPort)
	if err := m.Switch("metro", freePort(t)); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	m.Stop()
	if !canBind(proxyPort) {
		t.Fatalf("port %d still held after Stop", proxyPort)
	}
}
