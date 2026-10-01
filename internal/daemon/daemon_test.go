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

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/workspace"
)

type fakeTmux struct {
	gone atomic.Bool
	pid  atomic.Int64
}

func (t *fakeTmux) HasSession(string) bool    { return !t.gone.Load() }
func (t *fakeTmux) ServerPID() (int, error)   { return int(t.pid.Load()), nil }
func (t *fakeTmux) Tell(string, string) error { return nil }

type noWindows struct{}

func (noWindows) List() ([]trackwin.Info, error) { return nil, nil }
func (noWindows) Open(trackwin.Spec) (trackwin.Window, error) {
	return trackwin.Window{}, errors.New("no windows")
}
func (noWindows) Close(string) error { return nil }
func (noWindows) Respawn(string, trackwin.Spec) (trackwin.Window, error) {
	return trackwin.Window{}, errors.New("no tmux in this test")
}
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
	if _, err := os.Stat(filepath.Join(c.Home, ".claude", "skills", "tracks-v2-add-repo", "SKILL.md")); err != nil {
		t.Errorf("add-repo skill not installed: %v", err)
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
	if _, err := client.Promote(ctx, rpc.PromoteParams{ID: "20260928-101500-abc123"}, nil); !errors.As(err, &p) {
		t.Errorf("promoting a missing track: %v, want a problem", err)
	}
	if _, err := client.Restart(ctx, rpc.RestartParams{ID: "20260928-101500-abc123"}, nil); !errors.As(err, &p) {
		t.Errorf("restarting a missing track: %v, want a problem", err)
	}
	if _, err := client.AddRepo(ctx, rpc.AddRepoParams{ID: "20260928-101500-abc123", Repo: "web"}, nil); !errors.As(err, &p) {
		t.Errorf("adding a repo to a missing track: %v, want a problem", err)
	}
	if _, err := client.Archive(ctx, rpc.ArchiveParams{ID: "20260928-101500-abc123"}); !errors.As(err, &p) {
		t.Errorf("archiving a missing track: %v, want a problem", err)
	}
	if err := client.Unarchive(ctx, "20260928-101500-abc123"); !errors.As(err, &p) {
		t.Errorf("unarchiving a missing track: %v, want a problem", err)
	}
	for _, params := range []rpc.DerailParams{{Check: true}, {Force: true}} {
		params.ID = "20260928-101500-abc123"
		if _, err := client.Derail(ctx, params); !errors.As(err, &p) {
			t.Errorf("derailing a missing track with %+v: %v, want a problem", params, err)
		}
	}
	if _, err := client.Create(ctx, rpc.CreateParams{Request: tracks.Request{Kind: track.Ask, Prompt: "Why?", Engine: "nope"}}, nil); !errors.As(err, &p) {
		t.Fatalf("creating on an unknown engine: %v, want a problem", err)
	}
	listed, _, err := client.Station(ctx)
	if err != nil || len(listed) != 1 || listed[0].Draft == nil || listed[0].Status() != track.Draft || listed[0].Draft.Error != string(p) {
		t.Fatalf("Station = %+v, %v; want the failed creation's draft", listed, err)
	}
	if req, err := client.Draft(ctx, listed[0].ID); err != nil || req.Prompt != "Why?" || req.Draft != listed[0].ID {
		t.Errorf("Draft = %+v, %v", req, err)
	}
	if err := client.DiscardDraft(ctx, listed[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := client.DiscardDraft(ctx, listed[0].ID); !errors.As(err, &p) {
		t.Errorf("discarding a missing draft: %v, want a problem", err)
	}
	if _, err := client.Draft(ctx, listed[0].ID); !errors.As(err, &p) {
		t.Errorf("reading a missing draft: %v, want a problem", err)
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

func TestInterruptsOnStart(t *testing.T) {
	c, _ := config(t)
	ctx := context.Background()
	open := track.Track{ID: "20260928-101500-abc123", Kind: track.Ask, Name: "why", Engine: "claude", CreatedAt: time.Now()}
	if err := c.Tracks.Store.AddTrack(ctx, open); err != nil {
		t.Fatal(err)
	}
	c.Tracks.Worktrees = &workspace.Worktrees{Root: t.TempDir()}
	done := start(t, c)
	got, err := c.Tracks.Store.Track(ctx, open.ID)
	if err != nil || got.Open() || !got.Interrupted {
		t.Errorf("track = %+v, %v; want interrupted once the daemon answers", got.State, err)
	}
	client := rpc.Client{Socket: c.Paths.Socket}
	if listed, err := client.Interrupted(ctx); err != nil || len(listed) != 1 || listed[0].ID != open.ID {
		t.Errorf("Interrupted = %+v, %v; want the track", listed, err)
	}
	var steps []string
	reopened, err := client.Reopen(ctx, func(s string) { steps = append(steps, s) })
	if err != nil || len(reopened) != 1 || reopened[0].ID != open.ID || !strings.Contains(reopened[0].Error, "Engines tab") {
		t.Errorf("Reopen = %+v, %v; want it failing without Claude on the Engines tab", reopened, err)
	}
	if len(steps) != 1 || steps[0] != "Reopening why…" {
		t.Errorf("progress = %q", steps)
	}
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

func TestExitsWhenTheServerRestarts(t *testing.T) {
	c, tm := config(t)
	tm.pid.Store(100)
	done := start(t, c)
	time.Sleep(30 * time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("the daemon exited on its own server: %v", err)
	default:
	}
	tm.pid.Store(200)
	if err := wait(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestWatch(t *testing.T) {
	c, _ := config(t)
	changes := &tracks.Changes{}
	c.Tracks.Store, c.Tracks.Changes = tracks.Watched(c.Tracks.Store, changes), changes
	done := start(t, c)
	client := rpc.Client{Socket: c.Paths.Socket}
	ctx := context.Background()

	lines := make(chan struct{}, 10)
	watching := make(chan error, 1)
	go func() { watching <- client.Watch(ctx, func() { lines <- struct{}{} }) }()
	line := func(what string) {
		t.Helper()
		select {
		case <-lines:
		case <-time.After(5 * time.Second):
			t.Fatalf("no line %s", what)
		}
	}
	line("once connected")
	if err := client.SetFilter(ctx, track.Filter{Archived: true}); err != nil {
		t.Fatal(err)
	}
	line("after a change")

	if err := client.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if err := wait(t, done); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watching:
	case <-time.After(5 * time.Second):
		t.Fatal("Watch didn't return when the daemon exited")
	}
}
