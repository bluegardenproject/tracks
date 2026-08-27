package daemon

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
)

// testConfig is how a daemon test should build a config. It exists
// because config.Default() names the developer's own live tmux session
// ("tracks"), and a test that reaches a spawn lands real windows in it.
//
// That is not hypothetical. A test creating an ask-kind track — which
// is worktree-less, so it succeeds where the repo-dependent tests fail
// early — spawned two Claude sessions into the author's session, with
// the cwd falling back to their home directory because the test repo
// path didn't exist. Nothing in the test said "tmux"; the default did.
//
// The state dir is per-test too, so nothing touches the real store.
// The one deliberate exception is TestMaybeReloadConfigPreservesInfra,
// which needs a raw default to overwrite with sentinel values it never
// connects to.
func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Paths.StateDir = t.TempDir()
	cfg.Tmux.SessionName = testSessionName(t)
	// Belt to the braces: even a unique-looking name is only safe if
	// nothing is actually listening on it. Folded in here so the three
	// hand-rolled variants of this check don't drift apart.
	if tmux.New().HasSession(cfg.Tmux.SessionName) {
		t.Fatalf("tmux session %q already exists; refusing to run against a live session", cfg.Tmux.SessionName)
	}
	return cfg
}

// testSessionName derives a tmux session name unique to one test and
// impossible to confuse with a real one.
func testSessionName(t *testing.T) string {
	t.Helper()
	return "tracks-test-" + strings.NewReplacer("/", "-", " ", "-").Replace(t.Name())
}

// Checks what testConfig actually promises. Asserting the name differs
// from "tracks" would be a tautology — it is built by prefixing — so
// what's worth pinning is that the session doesn't exist and the state
// dir is isolated, which is what keeps a spawn off real infrastructure.
func TestTestConfigIsIsolatedFromRealInfrastructure(t *testing.T) {
	cfg := testConfig(t)

	if tmux.New().HasSession(cfg.Tmux.SessionName) {
		t.Errorf("session %q exists; a spawning test would land windows in it", cfg.Tmux.SessionName)
	}
	if cfg.Paths.StateDir == "" || cfg.Paths.StateDir == config.Default().Paths.StateDir {
		t.Errorf("StateDir = %q; startup GC would operate on the real store", cfg.Paths.StateDir)
	}
	if !strings.HasPrefix(cfg.Tmux.SessionName, "tracks-test-") {
		t.Errorf("session name %q is not recognisably a test session", cfg.Tmux.SessionName)
	}
}

// config.ModelKinds duplicates state.Kind so internal/config can stay
// independent of internal/state. This package imports both, so it is
// where the two are held together — adding a kind without teaching
// config about it would make a per-kind override for it fail
// validation.
func TestConfigModelKindsMatchStateKinds(t *testing.T) {
	stateKinds := []state.Kind{
		state.KindWork, state.KindReview, state.KindAsk, state.KindPlan, state.KindDoc,
	}
	want := make(map[string]bool, len(stateKinds))
	for _, k := range stateKinds {
		want[string(k)] = true
	}

	got := map[string]bool{}
	for _, k := range config.ModelKinds() {
		got[k] = true
		if !want[k] {
			t.Errorf("config.ModelKinds has %q, which is not a state.Kind", k)
		}
	}
	for k := range want {
		if !got[k] {
			t.Errorf("state.Kind %q is missing from config.ModelKinds — a per-kind override for it would be rejected", k)
		}
	}
}

// config.Providers duplicates state.Provider so internal/config can
// stay independent of internal/state. This package sees both, so it is
// where the two are held together.
func TestConfigProvidersMatchStateProviders(t *testing.T) {
	want := map[string]bool{}
	for _, p := range state.Providers() {
		want[string(p)] = true
	}
	got := map[string]bool{}
	for _, p := range config.Providers() {
		got[p] = true
		if !want[p] {
			t.Errorf("config.Providers has %q, which is not a state.Provider", p)
		}
		if !state.Provider(p).Valid() {
			t.Errorf("config offers %q but state rejects it as invalid", p)
		}
	}
	for p := range want {
		if !got[p] {
			t.Errorf("state.Provider %q missing from config.Providers — config.Validate would reject it", p)
		}
	}
}
