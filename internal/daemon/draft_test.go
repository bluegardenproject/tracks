package daemon

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// newDraftTestServer builds an isolated daemon whose only repo points at a
// nonexistent path, so every creation fails at the fetch step — the shape
// of an expired-token / network-drop failure. Used to exercise the
// failed-creation → draft flow without a real git remote.
func newDraftTestServer(t *testing.T) (*Server, *state.MemoryStore) {
	t.Helper()
	cfg := config.Default()
	cfg.Paths.StateDir = t.TempDir()
	cfg.Repos = []config.Repo{{Name: "demo", Path: "/nonexistent/demo", Base: "main"}}
	store := state.NewMemoryStore()
	return NewServer(cfg, store, "test"), store
}

// failNew runs a creation that is expected to fail and returns the
// persisted (errored) track plus the wire response.
func failNew(t *testing.T, srv *Server, store *state.MemoryStore) (state.Track, Response) {
	t.Helper()
	raw, err := json.Marshal(NewParams{Repos: []string{"demo"}, TaskPrompt: "fix the thing", Slug: "rate-bug", Kind: "work"})
	if err != nil {
		t.Fatal(err)
	}
	resp := srv.handleNew(context.Background(), raw, func(string) {})
	if resp.Ok {
		t.Fatal("expected creation to fail against a nonexistent repo, got ok")
	}
	tracks := store.All()
	if len(tracks) != 1 {
		t.Fatalf("expected 1 persisted track, got %d", len(tracks))
	}
	return tracks[0], resp
}

// TestFailedCreationCapturesDraftAndID verifies that a failed creation
// stores the user's parameters as a Draft on the errored track and that
// the wire response carries the track ID even on failure, so an
// interactive caller can offer to save it as a draft or dismiss it.
func TestFailedCreationCapturesDraftAndID(t *testing.T) {
	srv, store := newDraftTestServer(t)
	tr, resp := failNew(t, srv, store)

	if tr.Draft == nil {
		t.Fatal("errored track has no Draft spec; want the entered params captured")
	}
	if len(tr.Draft.Repos) != 1 || tr.Draft.Repos[0] != "demo" {
		t.Errorf("Draft.Repos = %v, want [demo]", tr.Draft.Repos)
	}
	if tr.Draft.TaskPrompt != "fix the thing" || tr.Draft.Slug != "rate-bug" {
		t.Errorf("Draft did not preserve prompt/slug: %+v", tr.Draft)
	}

	var nr NewResult
	if len(resp.Result) == 0 || json.Unmarshal(resp.Result, &nr) != nil {
		t.Fatalf("failure response carried no decodable result: %q", string(resp.Result))
	}
	if nr.TrackID != tr.ID {
		t.Errorf("result TrackID = %q, want the persisted track %q", nr.TrackID, tr.ID)
	}
}

// TestSaveDraftFlipsFailedTrackToDraft verifies handleSaveDraft turns a
// failed creation into a non-terminal draft that keeps its parameters and
// its failure reason.
func TestSaveDraftFlipsFailedTrackToDraft(t *testing.T) {
	srv, store := newDraftTestServer(t)
	tr, _ := failNew(t, srv, store)
	reason := tr.ErrorMsg

	raw, _ := json.Marshal(SaveDraftParams{ID: tr.ID})
	if resp := srv.handleSaveDraft(raw); !resp.Ok {
		t.Fatalf("save draft failed: %s", resp.Error)
	}

	got, ok := store.Get(tr.ID)
	if !ok {
		t.Fatal("track vanished after save draft")
	}
	if got.Status != state.StatusDraft {
		t.Errorf("status = %q, want %q", got.Status, state.StatusDraft)
	}
	if got.Status.IsTerminal() {
		t.Error("a draft must not be terminal")
	}
	if !got.CanLaunch() {
		t.Error("a saved draft should report CanLaunch")
	}
	if got.Draft == nil {
		t.Error("draft lost its params")
	}
	if got.ExitedAt != nil {
		t.Error("draft should have ExitedAt cleared")
	}
	if got.ErrorMsg != reason || reason == "" {
		t.Errorf("draft should keep the failure reason; got %q want %q", got.ErrorMsg, reason)
	}
}

// TestForgetDismissesDraft verifies a draft (which is non-terminal) can
// still be dismissed via Forget — the "Dismiss" choice.
func TestForgetDismissesDraft(t *testing.T) {
	srv, store := newDraftTestServer(t)
	tr, _ := failNew(t, srv, store)
	saveRaw, _ := json.Marshal(SaveDraftParams{ID: tr.ID})
	if resp := srv.handleSaveDraft(saveRaw); !resp.Ok {
		t.Fatalf("save draft failed: %s", resp.Error)
	}

	forgetRaw, _ := json.Marshal(ForgetParams{ID: tr.ID})
	if resp := srv.handleForget(forgetRaw); !resp.Ok {
		t.Fatalf("forget draft failed: %s", resp.Error)
	}
	if _, ok := store.Get(tr.ID); ok {
		t.Error("draft was not removed by forget")
	}
}

// TestLaunchFailureKeepsSingleDraft verifies that relaunching a draft
// whose creation fails again does not leave a duplicate: the throwaway
// errored record is dropped, the original draft is kept (still launchable)
// and its reason is refreshed.
func TestLaunchFailureKeepsSingleDraft(t *testing.T) {
	srv, store := newDraftTestServer(t)
	tr, _ := failNew(t, srv, store)
	saveRaw, _ := json.Marshal(SaveDraftParams{ID: tr.ID})
	if resp := srv.handleSaveDraft(saveRaw); !resp.Ok {
		t.Fatalf("save draft failed: %s", resp.Error)
	}

	launchRaw, _ := json.Marshal(LaunchParams{ID: tr.ID})
	resp := srv.handleLaunch(context.Background(), launchRaw, func(string) {})
	if resp.Ok {
		t.Fatal("expected relaunch against a nonexistent repo to fail")
	}

	tracks := store.All()
	if len(tracks) != 1 {
		t.Fatalf("expected exactly 1 track after a failed relaunch, got %d", len(tracks))
	}
	got := tracks[0]
	if got.ID != tr.ID {
		t.Errorf("kept track id = %q, want the original draft %q", got.ID, tr.ID)
	}
	if got.Status != state.StatusDraft {
		t.Errorf("original draft status = %q, want it to stay %q", got.Status, state.StatusDraft)
	}
	if !got.CanLaunch() {
		t.Error("draft should still be launchable after a failed relaunch")
	}
}

// TestEndTrackRefusesDraft guards the state-machine gap where ending a
// draft (which is non-terminal) would flip it to Done and make it
// prune-eligible, silently destroying the saved parameters. End/Kill must
// refuse a draft; it stays a launchable draft.
func TestEndTrackRefusesDraft(t *testing.T) {
	srv, store := newDraftTestServer(t)
	tr, _ := failNew(t, srv, store)
	saveRaw, _ := json.Marshal(SaveDraftParams{ID: tr.ID})
	if resp := srv.handleSaveDraft(saveRaw); !resp.Ok {
		t.Fatalf("save draft failed: %s", resp.Error)
	}

	doneRaw, _ := json.Marshal(DoneParams{ID: tr.ID})
	if resp := srv.handleDone(context.Background(), doneRaw, func(string) {}); resp.Ok {
		t.Fatal("expected End to refuse a draft, got ok")
	}
	if resp := srv.handleKill(context.Background(), doneRaw, func(string) {}); resp.Ok {
		t.Fatal("expected Kill to refuse a draft, got ok")
	}

	got, ok := store.Get(tr.ID)
	if !ok {
		t.Fatal("draft vanished after a refused End/Kill")
	}
	if got.Status != state.StatusDraft {
		t.Errorf("status = %q, want it to stay %q", got.Status, state.StatusDraft)
	}
	if !got.CanLaunch() {
		t.Error("draft should still be launchable after a refused End/Kill")
	}
}

func TestAuthHintPrefix(t *testing.T) {
	authy := []string{
		"fetch demo/main: fatal: Authentication failed for 'https://github.com/x/y.git/'",
		"remote: Permission to org/repo.git denied to user.",
		"git@github.com: Permission denied (publickey).",
		"fatal: could not read Username for 'https://github.com': terminal prompts disabled",
	}
	for _, m := range authy {
		if authHintPrefix(m) == "" {
			t.Errorf("expected an auth hint for %q", m)
		}
	}
	notAuth := []string{
		"fetch demo/main: fatal: couldn't find remote ref refs/heads/nope",
		"allocate ports: no free port block",
		"",
	}
	for _, m := range notAuth {
		if authHintPrefix(m) != "" {
			t.Errorf("did not expect an auth hint for %q", m)
		}
	}
}
