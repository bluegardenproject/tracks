package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// docTrackParams is the smallest creation that reaches the spawn: a doc
// review needs no repos, so nothing fails at the worktree step first.
// (Every pre-existing daemon test dies at that step, which is why the
// live-tmux escape went unnoticed for so long.)
func docTrackParams(t *testing.T) NewParams {
	t.Helper()
	doc := filepath.Join(t.TempDir(), "spec.md")
	if err := os.WriteFile(doc, []byte("# spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewParams{TaskPrompt: "review it", DocPath: doc}
}

// The regression test for Bug 7. Deliberately configures the real
// session name — "tracks", the one the user works in — and drives a
// creation all the way to the spawn. Before the seam this opened a real
// window running a real `claude` process there; now it must be refused.
//
// Note what failure costs: on a developer machine that has that session,
// a reverted seam makes this test open one real window before failing the
// assertion below, and nothing tears it down. That's the price of testing
// the actual production wiring rather than a stub of it — and it's a loud
// failure, not a silent one. On CI (no tmux server) it just fails.
func TestSpawnRefusedByDefaultEvenWithTheRealSessionConfigured(t *testing.T) {
	cfg := testConfig(t)
	// Deliberately restore the real session name — this test exists to
	// prove that even that is now inert. Everything else (state dir,
	// notifications) stays sandboxed.
	cfg.Tmux.SessionName = config.Default().Tmux.SessionName
	if cfg.Tmux.SessionName != "tracks" {
		t.Fatalf("premise changed: config.Default() session = %q, expected the real %q", cfg.Tmux.SessionName, "tracks")
	}
	store := state.NewMemoryStore()
	srv := NewServer(cfg, store, "test")

	raw, err := json.Marshal(docTrackParams(t))
	if err != nil {
		t.Fatal(err)
	}
	resp := srv.handleNew(context.Background(), raw, func(string) {})

	if resp.Ok {
		t.Fatal("creation succeeded — the spawn seam did not intercept, and this test just opened a real tmux window")
	}
	if !strings.Contains(resp.Error, "refusing to open a track window") {
		t.Errorf("error = %q, want the seam's refusal", resp.Error)
	}
	// The refusal must still land as a recoverable errored track carrying
	// its draft, exactly like any other spawn failure.
	tracks := store.All()
	if len(tracks) != 1 {
		t.Fatalf("expected the failed creation to be persisted, got %d tracks", len(tracks))
	}
	if tracks[0].Status != state.StatusErrored || tracks[0].Draft == nil {
		t.Errorf("track = %s with draft=%v, want errored with a draft", tracks[0].Status, tracks[0].Draft != nil)
	}
}

// The other half of the seam: a test that means to reach the spawn opts
// in, and gets the spawn recorded rather than performed. This is also
// the first coverage of a *successful* handleNew — previously untestable
// without touching real tmux.
func TestAllowSpawnRecordsInsteadOfSpawning(t *testing.T) {
	rec := allowSpawn(t)

	cfg := testConfig(t)
	store := state.NewMemoryStore()
	srv := NewServer(cfg, store, "test")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	params := docTrackParams(t)
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	resp := srv.handleNew(ctx, raw, func(string) {})
	if !resp.Ok {
		t.Fatalf("creation failed with the spawn allowed: %s", resp.Error)
	}
	// Stop the supervisor's watcher before asserting. Its first tick is
	// 2s out so it never lands in practice, but fakePID is deterministically
	// dead — so if one ever did land it would retire the track and flip the
	// status assertions below. On ctx.Done() the watcher only calls finish()
	// and returns; it never touches the store.
	cancel()

	calls := rec.Calls()
	if len(calls) != 1 {
		t.Fatalf("recorded %d spawns, want 1: %+v", len(calls), calls)
	}
	c := calls[0]
	if c.Kind != "window" {
		t.Errorf("spawn kind = %q, want window", c.Kind)
	}
	if !strings.HasPrefix(c.Target, cfg.Tmux.SessionName+":") {
		t.Errorf("spawn target = %q, want it in session %q", c.Target, cfg.Tmux.SessionName)
	}
	// The pane runs from the document's directory for a doc track, and
	// the command line carries the track id.
	if c.CWD != filepath.Dir(params.DocPath) {
		t.Errorf("spawn cwd = %q, want the document's dir %q", c.CWD, filepath.Dir(params.DocPath))
	}
	if !strings.Contains(c.Command, "TRACKS_ID=") {
		t.Errorf("spawn command = %q, want it to export TRACKS_ID", c.Command)
	}

	tracks := store.All()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}
	if tracks[0].Status != state.StatusRunning {
		t.Errorf("status = %s, want running", tracks[0].Status)
	}
	if tracks[0].PID != fakePID {
		t.Errorf("PID = %d, want the recorder's synthetic pid %d", tracks[0].PID, fakePID)
	}
}

// allowSpawn must not leak into the next test — otherwise one opt-in
// would quietly disarm the package's default-deny for everything after
// it. Verified by re-running the refusal after a sub-test opted in.
func TestAllowSpawnIsRestoredAfterTheTest(t *testing.T) {
	t.Run("opts in", func(t *testing.T) {
		allowSpawn(t)
		if _, err := spawnTrackWindow("s", "w", "cmd", "/tmp"); err != nil {
			t.Fatalf("allowed spawn should succeed: %v", err)
		}
	})
	if _, err := spawnTrackWindow("s", "w", "cmd", "/tmp"); err == nil {
		t.Error("spawn still allowed after the opting-in sub-test finished")
	}
}

// Every seam refuses by default, not just the track window — a dev-server
// pane runs the repo's install and start commands, which is at least as
// consequential as opening a window.
func TestAllSpawnSeamsRefuseByDefault(t *testing.T) {
	if _, err := spawnTrackWindow("s", "w", "cmd", "/tmp"); err == nil {
		t.Error("spawnTrackWindow should refuse by default")
	}
	if _, _, err := spawnServicePaneRight("s", "w", "cmd", "/tmp", 30); err == nil {
		t.Error("spawnServicePaneRight should refuse by default")
	}
	if _, _, err := spawnServicePaneBelow(fakePaneFirst, "cmd", "/tmp"); err == nil {
		t.Error("spawnServicePaneBelow should refuse by default")
	}
}

// The direct calls above would pass even if services.go bypassed the
// seams, so drive the production path: openServerPane must go through
// spawnServicePaneRight for a track's first dev server and
// spawnServicePaneBelow for the next one. Reverting either call site in
// services.go must fail here.
func TestOpenServerPaneRoutesThroughTheSeams(t *testing.T) {
	rec := allowSpawn(t)
	cfg := testConfig(t)
	srv := NewServer(cfg, state.NewMemoryStore(), "test")
	sup := &supervisor{trackID: "tid", windowName: "win"}

	pid, err := srv.openServerPane(sup, "web", 3000, "pnpm dev", "/tmp/wt")
	if err != nil {
		t.Fatalf("first service pane: %v", err)
	}
	if pid != fakePID+1 {
		t.Errorf("first pane pid = %d, want %d", pid, fakePID+1)
	}
	if _, err := srv.openServerPane(sup, "api", 3001, "pnpm api", "/tmp/wt"); err != nil {
		t.Fatalf("second service pane: %v", err)
	}

	calls := rec.Calls()
	if len(calls) != 2 {
		t.Fatalf("recorded %d spawns, want 2: %+v", len(calls), calls)
	}
	if calls[0].Kind != "pane-right" {
		t.Errorf("first spawn kind = %q, want pane-right", calls[0].Kind)
	}
	if calls[0].Target != cfg.Tmux.SessionName+":win" || calls[0].Command != "pnpm dev" || calls[0].CWD != "/tmp/wt" {
		t.Errorf("first spawn = %+v, want the track window / command / worktree", calls[0])
	}
	// The second stacks under the first, so its target is that pane's id.
	if calls[1].Kind != "pane-below" {
		t.Errorf("second spawn kind = %q, want pane-below", calls[1].Kind)
	}
	if calls[1].Target != fakePaneFirst {
		t.Errorf("second spawn target = %q, want the first pane %q", calls[1].Target, fakePaneFirst)
	}
	if sup.lastServicePane != fakePaneSecond {
		t.Errorf("lastServicePane = %q, want %q", sup.lastServicePane, fakePaneSecond)
	}
}
