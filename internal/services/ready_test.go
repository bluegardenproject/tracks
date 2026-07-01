package services

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestProbeIsZero(t *testing.T) {
	if !(Probe{}).IsZero() {
		t.Error("empty probe should be zero")
	}
	if (Probe{Port: "1"}).IsZero() {
		t.Error("port probe should not be zero")
	}
	if (Probe{LogRegex: "x"}).IsZero() {
		t.Error("log probe should not be zero")
	}
}

func TestWaitReadyZeroProbeReturnsImmediately(t *testing.T) {
	if err := WaitReady(context.Background(), Probe{}, "", time.Second); err != nil {
		t.Errorf("zero probe should be instantly ready: %v", err)
	}
}

func TestWaitReadyPort(t *testing.T) {
	// A live listener satisfies a port probe.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	if err := WaitReady(context.Background(), Probe{Port: port}, "", 2*time.Second); err != nil {
		t.Errorf("expected ready on open port %s: %v", port, err)
	}
}

func TestWaitReadyPortTimesOut(t *testing.T) {
	// Grab then release a port so it's a real, closed one.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()

	start := time.Now()
	err = WaitReady(context.Background(), Probe{Port: port}, "", 300*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout on closed port")
	}
	if time.Since(start) > 3*time.Second {
		t.Errorf("timeout took too long: %s", time.Since(start))
	}
}

func TestWaitReadyLogRegex(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "svc.log")
	if err := os.WriteFile(logPath, []byte("booting...\nnow listening on :3000\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WaitReady(context.Background(), Probe{LogRegex: `listening on`}, logPath, 2*time.Second); err != nil {
		t.Errorf("expected log match: %v", err)
	}
}

func TestWaitReadyBadLogRegex(t *testing.T) {
	if err := WaitReady(context.Background(), Probe{LogRegex: "("}, "", time.Second); err == nil {
		t.Error("expected error on invalid regex")
	}
}

func TestWaitReadyCancel(t *testing.T) {
	// A cancelled context aborts the wait promptly.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := WaitReady(ctx, Probe{Port: port}, "", 5*time.Second); err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestPortOpen(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	if !portOpen(port) {
		t.Error("open listener should report portOpen=true")
	}
	ln.Close()
	// Asserting the just-closed port is immediately unopenable is racy
	// across OSes (TIME_WAIT / lingering accept), so we don't. Port 1 is
	// a reliable never-open case in the test environment.
	if portOpen(strconv.Itoa(1)) {
		t.Error("port 1 should not be open in tests")
	}
}
