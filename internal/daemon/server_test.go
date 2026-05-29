package daemon

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// makeServer wires up a Server suitable for in-process tests:
// in-memory state, socket dir under t.TempDir, and the tmux watch
// loop disabled. Returns the Server, a Client connected to it, and
// a cleanup func.
func makeServer(t *testing.T) (*Server, *Client, func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Paths.SocketDir = dir
	cfg.Repos = []config.Repo{
		{Name: "demo", Path: "/nonexistent/demo", Base: "main"},
	}
	st := state.NewMemoryStore()
	srv := NewServer(cfg, st, "test-version")
	srv.NoTmuxWatch = true

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = srv.Start(ctx)
		close(done)
	}()

	// Wait briefly for the socket to come up.
	socketPath := filepath.Join(dir, "sock")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := exec.LookPath("test"); err == nil {
			// noop, just to use exec import; real check below
		}
		if fileExists(socketPath) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	cl := NewClient(cfg)
	cl.DialTimeout = 200 * time.Millisecond

	cleanup := func() {
		cancel()
		srv.Stop()
		<-done
	}
	return srv, cl, cleanup
}

func fileExists(p string) bool {
	cmd := exec.Command("test", "-S", p)
	return cmd.Run() == nil
}

func TestPing(t *testing.T) {
	_, cl, cleanup := makeServer(t)
	defer cleanup()
	r, err := cl.Ping()
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "test-version" || r.PID == 0 {
		t.Errorf("ping: %+v", r)
	}
}

func TestLsEmpty(t *testing.T) {
	_, cl, cleanup := makeServer(t)
	defer cleanup()
	got, err := cl.Ls()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %d", len(got))
	}
}

func TestGetMissing(t *testing.T) {
	_, cl, cleanup := makeServer(t)
	defer cleanup()
	_, found, err := cl.Get("nope")
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Error("missing track should not be found")
	}
}

func TestSecondDaemonRefused(t *testing.T) {
	srv1, _, cleanup := makeServer(t)
	defer cleanup()

	// A second server with the same socket dir must refuse to start.
	srv2 := NewServer(srv1.cfg, state.NewMemoryStore(), "v2")
	srv2.NoTmuxWatch = true
	err := srv2.Start(context.Background())
	if err == nil {
		t.Fatal("expected second daemon to fail startup")
	}
}

func TestShutdownExitsCleanly(t *testing.T) {
	srv, cl, _ := makeServer(t)
	if err := cl.Shutdown(); err != nil {
		t.Fatal(err)
	}
	// Server.Stop is goroutine-driven from inside handleConn after
	// the response is written. Give it a moment.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if srv.stopped.Load() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("daemon did not stop after shutdown")
}

func TestParallelRequests(t *testing.T) {
	_, cl, cleanup := makeServer(t)
	defer cleanup()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := cl.Ping(); err != nil {
				t.Errorf("ping: %v", err)
			}
		}()
	}
	wg.Wait()
}
