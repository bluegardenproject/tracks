package daemon

import (
	"context"
	"os"
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

// TestCancelRootContextShutsDownDaemon guards the regression where a
// cancelled root context (SIGINT/SIGTERM via main's signal.NotifyContext,
// or a stray signal to the inherited process group) left the daemon
// wedged: acceptLoop kept blocking in Accept() so Start never returned,
// and every request handled with the dead ctx failed with "context
// canceled". Start must return promptly once ctx is cancelled.
func TestCancelRootContextShutsDownDaemon(t *testing.T) {
	// A short temp dir, not t.TempDir(): this test's long name would push
	// the "<dir>/sock" unix-socket path past macOS's ~104-byte sun_path
	// limit, making Listen fail with "bind: invalid argument".
	dir, err := os.MkdirTemp("", "trkd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	cfg := config.Default()
	cfg.Paths.SocketDir = dir
	st := state.NewMemoryStore()
	srv := NewServer(cfg, st, "test-version")
	srv.NoTmuxWatch = true

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = srv.Start(ctx)
		close(done)
	}()

	// Wait until the listener is bound and the accept loop is running,
	// so cancel() exercises the running-daemon path rather than a race
	// during early startup.
	deadline := time.Now().Add(2 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		srv.mu.Lock()
		bound := srv.listener != nil
		srv.mu.Unlock()
		if bound {
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("daemon listener never came up")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after root context was cancelled — daemon wedged")
	}
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
	srv2 := NewServer(srv1.config(), state.NewMemoryStore(), "v2")
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

// newServerWithConfigFile points config.Path() at a temp XDG dir,
// writes cfg there, and returns a Server loaded from it with its
// reload baseline initialized — the same state a freshly started
// daemon is in. No socket is opened.
func newServerWithConfigFile(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if _, err := config.Save(cfg); err != nil {
		t.Fatalf("save initial config: %v", err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("load initial config: %v", err)
	}
	srv := NewServer(loaded, state.NewMemoryStore(), "test-version")
	srv.initConfigWatch()
	return srv
}

func TestMaybeReloadConfigPicksUpNewRepo(t *testing.T) {
	cfg := config.Default()
	cfg.Repos = []config.Repo{{Name: "demo", Path: "/x/demo", Base: "main"}}
	srv := newServerWithConfigFile(t, cfg)

	if _, ok := srv.config().RepoByName("added"); ok {
		t.Fatal("repo 'added' present before it was configured")
	}

	// Edit the file: add a repo. Size changes, so the reload triggers
	// even if the mtime resolution can't distinguish the two writes.
	cfg.Repos = append(cfg.Repos, config.Repo{Name: "added", Path: "/x/added", Base: "main"})
	if _, err := config.Save(cfg); err != nil {
		t.Fatalf("save edited config: %v", err)
	}

	srv.maybeReloadConfig()

	if _, ok := srv.config().RepoByName("added"); !ok {
		t.Fatal("repo 'added' not visible after reload")
	}
}

func TestMaybeReloadConfigPreservesInfra(t *testing.T) {
	cfg := config.Default()
	cfg.Tmux.SessionName = "orig-session"
	cfg.Paths.StateDir = "/orig/state"
	srv := newServerWithConfigFile(t, cfg)

	// A live edit to startup-bound infra must be ignored, not honored.
	cfg.Tmux.SessionName = "changed-session"
	cfg.Paths.StateDir = "/changed/state"
	cfg.Repos = append(cfg.Repos, config.Repo{Name: "added", Path: "/x/added", Base: "main"})
	if _, err := config.Save(cfg); err != nil {
		t.Fatalf("save edited config: %v", err)
	}

	srv.maybeReloadConfig()

	got := srv.config()
	if got.Tmux.SessionName != "orig-session" {
		t.Errorf("session name = %q, want preserved 'orig-session'", got.Tmux.SessionName)
	}
	if got.Paths.StateDir != "/orig/state" {
		t.Errorf("state dir = %q, want preserved '/orig/state'", got.Paths.StateDir)
	}
	// But non-infra edits in the same write still land.
	if _, ok := got.RepoByName("added"); !ok {
		t.Error("repo 'added' not visible after reload")
	}
}

func TestMaybeReloadConfigKeepsPreviousOnParseError(t *testing.T) {
	cfg := config.Default()
	cfg.Repos = []config.Repo{{Name: "demo", Path: "/x/demo", Base: "main"}}
	srv := newServerWithConfigFile(t, cfg)

	// Clobber the file with invalid YAML.
	p, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("repos: [this is not: valid"), 0o600); err != nil {
		t.Fatalf("write bad config: %v", err)
	}

	srv.maybeReloadConfig()

	// The good previous config must survive a malformed edit.
	if _, ok := srv.config().RepoByName("demo"); !ok {
		t.Fatal("previous config lost after a malformed edit")
	}
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
