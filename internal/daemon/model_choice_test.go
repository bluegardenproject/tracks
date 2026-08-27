package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/claude"
	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// newModelTestServer mirrors newDraftTestServer: the repo path does not
// exist, so creation fails at the fetch step. The track record is
// written before provisioning, which is what these assertions read —
// the model has to be resolved and stored by then, or a promote and a
// relaunch would both lose it.
func newModelTestServer(t *testing.T, claudeCfg config.Claude) (*Server, *state.MemoryStore) {
	t.Helper()
	cfg := testConfig(t)
	cfg.Repos = []config.Repo{{Name: "demo", Path: "/nonexistent/demo", Base: "main"}}
	cfg.Claude = claudeCfg
	store := state.NewMemoryStore()
	return NewServer(cfg, store, "test"), store
}

func createWithParams(t *testing.T, srv *Server, store *state.MemoryStore, p NewParams) state.Track {
	t.Helper()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if resp := srv.handleNew(context.Background(), raw, func(string) {}); resp.Ok {
		t.Fatal("expected creation to fail against a nonexistent repo, got ok")
	}
	tracks := store.All()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 persisted track, got %d", len(tracks))
	}
	return tracks[0]
}

func TestCreateStoresThePickedModel(t *testing.T) {
	srv, store := newModelTestServer(t, config.Claude{Model: "claude-opus-5"})
	tr := createWithParams(t, srv, store, NewParams{
		Repos: []string{"demo"}, TaskPrompt: "do it", Kind: "work",
		Model: "claude-opus-4-8",
	})
	if tr.RequestedModel != "claude-opus-4-8" {
		t.Errorf("RequestedModel = %q, want the caller's pick", tr.RequestedModel)
	}
	// The draft has to carry it too, or relaunching a failed creation
	// silently swaps the model out from under the user.
	if tr.Draft == nil {
		t.Fatal("errored track has no draft")
	}
	if tr.Draft.Model != "claude-opus-4-8" {
		t.Errorf("Draft.Model = %q, want the pick preserved for relaunch", tr.Draft.Model)
	}
}

// "No preference" must stay unresolved on the record. Baking the
// current default in here would survive a kind change, and promote
// changes the kind — see TestPromotedTrackFollowsTheWorkDefault.
func TestCreateDoesNotFreezeTheDefault(t *testing.T) {
	claude := config.Claude{
		Model:       "claude-opus-5",
		ModelByKind: map[string]string{"ask": "haiku"},
	}
	srv, store := newModelTestServer(t, claude)
	tr := createWithParams(t, srv, store, NewParams{
		Repos: []string{"demo"}, TaskPrompt: "look at it", Kind: "work",
	})
	if tr.RequestedModel != "" {
		t.Errorf("RequestedModel = %q, want empty — no model was picked, so the default is resolved at spawn", tr.RequestedModel)
	}
	if tr.Draft != nil && tr.Draft.Model != "" {
		t.Errorf("Draft.Model = %q, want empty for the same reason", tr.Draft.Model)
	}
}

func TestCreateLeavesTheModelEmptyWhenNoneConfigured(t *testing.T) {
	srv, store := newModelTestServer(t, config.Claude{Binary: "claude"})
	tr := createWithParams(t, srv, store, NewParams{
		Repos: []string{"demo"}, TaskPrompt: "do it", Kind: "work",
	})
	if tr.RequestedModel != "" {
		t.Errorf("RequestedModel = %q, want empty so Claude's own default applies", tr.RequestedModel)
	}
}

// The consequence of the bug, at the spawn boundary. An ask track the
// user picked no model for, promoted to work, must run the *work*
// default — promote flips Kind and re-spawns, so a default resolved at creation time
// would carry the ask model into the work session.
//
// The record is built directly rather than through handleNew: an ask
// track is worktree-less, so its creation succeeds and would spawn a
// real tmux window. That means this test would still pass if handleNew
// regressed to freezing the default — TestCreateDoesNotFreezeTheDefault
// is what holds that line. This one covers the other half: that
// BuildOptions resolves against the post-flip kind.
func TestPromotedTrackFollowsTheWorkDefault(t *testing.T) {
	cfg := testConfig(t)
	cfg.Claude.Model = "claude-opus-5"
	cfg.Claude.ModelByKind = map[string]string{"ask": "haiku", "work": "claude-opus-4-8"}

	// An ask track created with no preference, after handlePromote has
	// flipped it to work.
	promoted := state.Track{
		ID:             "20260101-000000-abcdef",
		Kind:           state.KindWork,
		TaskPrompt:     "how does this work?",
		RequestedModel: "", // no pick — the whole point
		Repos:          []state.TrackRepo{{Name: "demo", Path: t.TempDir()}},
	}

	opts, err := claude.BuildOptions(cfg, promoted, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Model != "claude-opus-4-8" {
		t.Errorf("promoted session runs %q, want the work default claude-opus-4-8 (%q would be the ask default leaking through)",
			opts.Model, "haiku")
	}
}

// An explicit pick is the opposite case: it must survive the promote
// untouched, because the user did choose it.
func TestPromotedTrackKeepsAnExplicitPick(t *testing.T) {
	cfg := testConfig(t)
	cfg.Claude.ModelByKind = map[string]string{"ask": "haiku", "work": "claude-opus-4-8"}

	promoted := state.Track{
		ID:             "20260101-000000-abcdef",
		Kind:           state.KindWork,
		TaskPrompt:     "q",
		RequestedModel: "claude-sonnet-5",
		Repos:          []state.TrackRepo{{Name: "demo", Path: t.TempDir()}},
	}

	opts, err := claude.BuildOptions(cfg, promoted, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Model != "claude-sonnet-5" {
		t.Errorf("promoted session runs %q, want the explicitly picked claude-sonnet-5", opts.Model)
	}
}

// Provider, unlike the model, is resolved at creation: the spawn path
// picks a binary from it, and a track whose provider drifted would try
// to resume a Cursor chat id with `claude --resume`.
func TestCreateResolvesTheProvider(t *testing.T) {
	srv, store := newModelTestServer(t, config.Claude{Binary: "claude"})
	tr := createWithParams(t, srv, store, NewParams{
		Repos: []string{"demo"}, TaskPrompt: "do it", Kind: "work",
		Provider: "cursor",
	})
	if tr.Provider != state.ProviderCursor {
		t.Errorf("Provider = %q, want the caller's pick", tr.Provider)
	}
	if tr.Draft == nil || tr.Draft.Provider != state.ProviderCursor {
		t.Errorf("draft did not preserve the provider for relaunch: %+v", tr.Draft)
	}
}

// No pick means the configured default, materialised onto the record
// rather than left empty — see the comment at handleNew.
func TestCreateAppliesTheDefaultProvider(t *testing.T) {
	srv, store := newModelTestServer(t, config.Claude{Binary: "claude"})
	cfg := srv.config()
	cfg.Provider = "cursor"
	srv.cfg.Store(&cfg)

	tr := createWithParams(t, srv, store, NewParams{
		Repos: []string{"demo"}, TaskPrompt: "do it", Kind: "work",
	})
	if tr.Provider != state.ProviderCursor {
		t.Errorf("Provider = %q, want the configured default applied", tr.Provider)
	}
}

// A default of "" (or a config that never mentions providers) has to
// come out as Claude, not as an empty provider the spawn path can't use.
func TestCreateFallsBackToClaude(t *testing.T) {
	srv, store := newModelTestServer(t, config.Claude{Binary: "claude"})
	cfg := srv.config()
	cfg.Provider = ""
	srv.cfg.Store(&cfg)

	tr := createWithParams(t, srv, store, NewParams{
		Repos: []string{"demo"}, TaskPrompt: "do it", Kind: "work",
	})
	if tr.Provider != state.ProviderClaude {
		t.Errorf("Provider = %q, want claude", tr.Provider)
	}
}

// The design's "done when" for this phase requires the draft round
// trip, not just that the draft holds the value. These call
// draftLaunchParams — the function handleLaunch itself uses — rather
// than rebuilding the mapping, which is what made the first version of
// this test pass with the production line deleted.
func TestDraftLaunchParamsCarriesTheProvider(t *testing.T) {
	tr := state.Track{Draft: &state.DraftSpec{
		Repos: []string{"demo"}, TaskPrompt: "do it", Kind: "work",
		Provider: state.ProviderCursor, Model: "gpt-5.3-codex",
	}}
	got := draftLaunchParams(tr)
	if got.Provider != string(state.ProviderCursor) {
		t.Errorf("Provider = %q, want cursor", got.Provider)
	}
	if got.Model != "gpt-5.3-codex" {
		t.Errorf("Model = %q", got.Model)
	}
}

// A draft written before providers existed carries "". It must replay
// as Claude — the provider it was created under — not as whatever the
// default has become since.
func TestLegacyDraftLaunchesAsClaude(t *testing.T) {
	tr := state.Track{Draft: &state.DraftSpec{
		Repos: []string{"demo"}, TaskPrompt: "old work", Kind: "work",
	}}
	if got := draftLaunchParams(tr).Provider; got != string(state.ProviderClaude) {
		t.Errorf("legacy draft replays as %q, want claude — it would otherwise pick up the current default", got)
	}
}

// The end-to-end path still has to reach handleNew; this pins that the
// two are actually connected, which the unit tests above cannot.
func TestRelaunchReachesCreation(t *testing.T) {
	srv, store := newModelTestServer(t, config.Claude{Binary: "claude"})
	tr := createWithParams(t, srv, store, NewParams{
		Repos: []string{"demo"}, TaskPrompt: "do it", Kind: "work", Provider: "cursor",
	})
	raw, err := json.Marshal(LaunchParams{ID: tr.ID})
	if err != nil {
		t.Fatal(err)
	}
	if resp := srv.handleLaunch(context.Background(), raw, func(string) {}); resp.Ok {
		t.Fatal("expected the relaunch to fail against a nonexistent repo")
	}
	var sawAttempt bool
	for _, c := range store.All() {
		if c.ErrorMsg != "" {
			sawAttempt = true
		}
	}
	if !sawAttempt {
		t.Error("relaunch never reached creation — no track carries a failure")
	}
}

// An unknown provider must be refused rather than persisted onto a
// field documented as fixed for the life of the track.
func TestCreateRejectsAnUnknownProvider(t *testing.T) {
	srv, store := newModelTestServer(t, config.Claude{Binary: "claude"})
	raw, err := json.Marshal(NewParams{
		Repos: []string{"demo"}, TaskPrompt: "do it", Kind: "work", Provider: "codex",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := srv.handleNew(context.Background(), raw, func(string) {})
	if resp.Ok {
		t.Fatal("an unknown provider was accepted")
	}
	if !strings.Contains(resp.Error, "codex") {
		t.Errorf("error should name the offending value, got: %q", resp.Error)
	}
	if len(store.All()) != 0 {
		t.Errorf("a track was persisted for a rejected provider: %+v", store.All())
	}
}
