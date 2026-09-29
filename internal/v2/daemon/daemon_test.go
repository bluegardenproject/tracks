package daemon

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/rpc"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
)

type fakeTmux struct{ gone atomic.Bool }

func (t *fakeTmux) HasSession(string) bool    { return !t.gone.Load() }
func (t *fakeTmux) Tell(string, string) error { return nil }

type noWindows struct{}

func (noWindows) List() ([]trackwin.Info, error) { return nil, nil }
func (noWindows) Open(trackwin.Spec) (trackwin.Window, error) {
	return trackwin.Window{}, errors.New("no windows")
}
func (noWindows) Close(string) error            { return nil }
func (noWindows) Attention(string, bool) error  { return nil }
func (noWindows) Screen(string) (string, error) { return "", nil }

// config puts the daemon's files in a short folder: a Unix socket path
// has to stay under about 100 bytes.
func config(t *testing.T) (Config, *fakeTmux) {
	dir, err := os.MkdirTemp("/tmp", "tv2d-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	st, err := store.Open(context.Background(), filepath.Join(dir, "tracks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	tm := &fakeTmux{}
	return Config{
		Paths: platform.Paths{
			DataDir: dir, Socket: filepath.Join(dir, "daemon.sock"), Lock: filepath.Join(dir, "daemon.lock"),
			BinDir: filepath.Join(dir, "bin"),
		},
		Version: "v-test", Tracks: &tracks.Service{Store: st, Windows: noWindows{}, Settings: func() (settings.Settings, error) { return settings.Settings{}, nil }}, Tmux: tm, Session: "tracks",
		Home: filepath.Join(dir, "home"), Log: log.New(io.Discard, "", 0), Every: 10 * time.Millisecond,
	}, tm
}

func start(t *testing.T, c Config) <-chan error {
	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), c) }()
	client := rpc.Client{Socket: c.Paths.Socket}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if _, err := client.Ping(context.Background()); err == nil {
			return done
		}
	}
	t.Fatal("the daemon didn't answer")
	return nil
}

func wait(t *testing.T, done <-chan error) error {
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the daemon didn't exit")
		return nil
	}
}

func TestDaemon(t *testing.T) {
	c, _ := config(t)
	done := start(t, c)
	client := rpc.Client{Socket: c.Paths.Socket}
	ctx := context.Background()

	r, err := client.Ping(ctx)
	if err != nil || r.Version != "v-test" || r.PID != os.Getpid() || r.Exe == "" || r.ExeModified == 0 {
		t.Errorf("Ping = %+v, %v", r, err)
	}
	if err := Run(ctx, c); !errors.Is(err, ErrRunning) {
		t.Errorf("a second daemon: %v, want ErrRunning", err)
	}
	if _, err := client.Ping(ctx); err != nil {
		t.Errorf("the first daemon stopped answering: %v", err)
	}

	shim, err := os.ReadFile(filepath.Join(c.Paths.BinDir, "tracks"))
	if err != nil || !strings.Contains(string(shim), `--new-app "$@"`) {
		t.Errorf("tracks command = %q, %v", shim, err)
	}
	if _, err := os.Stat(filepath.Join(c.Home, ".claude", "agents", "tracks-v2-reviewer.md")); err != nil {
		t.Errorf("reviewer not installed: %v", err)
	}
	if info, err := os.Stat(c.Paths.Socket); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("socket mode: %v, %v", info.Mode(), err)
	}
	// Long enough for the windows to be checked a few times.
	time.Sleep(50 * time.Millisecond)
	if listed, err := client.List(ctx); err != nil || len(listed) != 0 {
		t.Errorf("List = %v, %v", listed, err)
	}
	var p tracks.Problem
	if _, err := client.Resume(ctx, rpc.ResumeParams{ID: "20260928-101500-abc123"}, nil); !errors.As(err, &p) {
		t.Errorf("resuming a missing track: %v, want a problem", err)
	}
	if err := client.Report(ctx, rpc.ReportParams{ID: "20260928-101500-abc123", Event: "agent.waiting"}); !errors.As(err, &p) {
		t.Errorf("reporting on a missing track: %v, want a problem", err)
	}
	for _, params := range []rpc.CleanParams{{Check: true}, {Force: true}} {
		params.ID = "20260928-101500-abc123"
		if _, err := client.Clean(ctx, params); !errors.As(err, &p) {
			t.Errorf("cleaning a missing track with %+v: %v, want a problem", params, err)
		}
	}
	if _, err := client.Archive(ctx, rpc.ArchiveParams{ID: "20260928-101500-abc123"}); !errors.As(err, &p) {
		t.Errorf("archiving a missing track: %v, want a problem", err)
	}
	if err := client.Unarchive(ctx, "20260928-101500-abc123"); !errors.As(err, &p) {
		t.Errorf("unarchiving a missing track: %v, want a problem", err)
	}
	if err := client.SetFilter(ctx, track.Filter{Started: track.Between}); !errors.As(err, &p) {
		t.Errorf("an invalid filter: %v, want a problem", err)
	}
	want := track.Filter{Archived: true, Started: track.Last7Days}
	if err := client.SetFilter(ctx, want); err != nil {
		t.Fatal(err)
	}
	if f, err := client.Filter(ctx); err != nil || f.String() != want.String() {
		t.Errorf("Filter = %+v, %v; want %+v", f, err, want)
	}
	if listed, f, err := client.Station(ctx); err != nil || len(listed) != 0 || f.String() != want.String() {
		t.Errorf("Station = %v, %+v, %v", listed, f, err)
	}

	if err := client.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := wait(t, done); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(c.Paths.Socket); !os.IsNotExist(err) {
		t.Errorf("the socket is still there: %v", err)
	}

	// A socket left behind by a daemon that died is replaced.
	if err := os.WriteFile(c.Paths.Socket, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	done = start(t, c)
	if err := client.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := wait(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestExitsWithTheSession(t *testing.T) {
	c, tm := config(t)
	done := start(t, c)
	tm.gone.Store(true)
	if err := wait(t, done); err != nil {
		t.Fatal(err)
	}
}
