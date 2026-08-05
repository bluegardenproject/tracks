package daemon

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/state"
)

func mustParams(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	return b
}

// With no IDs, reopen targets every interrupted track and nothing else,
// oldest first so the reopened windows land in creation order.
func TestReopenTargetsAllInterruptedOldestFirst(t *testing.T) {
	srv := newQuietServer(t)
	base := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	for _, tr := range []state.Track{
		{ID: "newer", Status: state.StatusInterrupted, CreatedAt: base.Add(2 * time.Hour)},
		{ID: "older", Status: state.StatusInterrupted, CreatedAt: base},
		{ID: "middle", Status: state.StatusInterrupted, CreatedAt: base.Add(time.Hour)},
		{ID: "done", Status: state.StatusDone, CreatedAt: base},
		{ID: "running", Status: state.StatusRunning, CreatedAt: base},
		{ID: "draft", Status: state.StatusDraft, CreatedAt: base},
		{ID: "review", Status: state.StatusPR, CreatedAt: base},
	} {
		if err := srv.store.Put(tr); err != nil {
			t.Fatalf("put %s: %v", tr.ID, err)
		}
	}

	targets, err := srv.reopenTargets(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got []string
	for _, tr := range targets {
		got = append(got, tr.ID)
	}
	want := []string{"older", "middle", "newer"}
	if len(got) != len(want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("targets = %v, want %v", got, want)
		}
	}
}

// Explicit IDs are validated: reopen is for interrupted tracks, and
// pointing it at anything else is a mistake worth an error rather than a
// silent skip.
func TestReopenTargetsRejectsIneligibleIDs(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{ID: "done", Status: state.StatusDone}); err != nil {
		t.Fatalf("put: %v", err)
	}

	if _, err := srv.reopenTargets([]string{"ghost"}); err == nil {
		t.Error("unknown track id: want an error")
	}
	if _, err := srv.reopenTargets([]string{"done"}); err == nil {
		t.Error("finished track: want an error")
	}
}

// A track from before session tracking can't be reopened. It must be
// reported as a failure and left interrupted — not silently dropped, and
// not flipped to errored.
func TestReopenReportsTrackWithoutSessionID(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{ID: "no-session", Status: state.StatusInterrupted}); err != nil {
		t.Fatalf("put: %v", err)
	}

	resp := srv.handleReopen(context.Background(), nil, func(string) {})
	if !resp.Ok {
		t.Fatalf("handleReopen failed: %s", resp.Error)
	}
	var res ReopenResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(res.Reopened) != 0 {
		t.Errorf("Reopened = %v, want none", res.Reopened)
	}
	if len(res.Failed) != 1 || res.Failed[0].ID != "no-session" {
		t.Fatalf("Failed = %+v, want one entry for no-session", res.Failed)
	}
	if got, _ := srv.store.Get("no-session"); got.Status != state.StatusInterrupted {
		t.Errorf("status = %q, want it left at %q", got.Status, state.StatusInterrupted)
	}
}

// A track whose worktree can't be restored stays interrupted, so the
// user can fix the cause (a deleted branch, a repo dropped from config)
// and reopen again. Only a failed *spawn* marks a track errored.
func TestReopenWorktreeFailureKeepsTrackInterrupted(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:        "orphan-repo",
		Status:    state.StatusInterrupted,
		SessionID: "22222222-2222-2222-2222-222222222222",
		Kind:      state.KindWork,
		Branch:    "feat/gone",
		// Not in the (default, empty) config, and its path doesn't exist,
		// so worktree restoration fails before anything is spawned.
		Repos: []state.TrackRepo{{Name: "ghost", Path: "/nonexistent/ghost", Branch: "feat/gone"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	resp := srv.handleReopen(context.Background(),
		mustParams(t, ReopenParams{IDs: []string{"orphan-repo"}}), func(string) {})
	if !resp.Ok {
		t.Fatalf("handleReopen failed: %s", resp.Error)
	}
	var res ReopenResult
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(res.Failed) != 1 {
		t.Fatalf("Failed = %+v, want one entry", res.Failed)
	}
	got, _ := srv.store.Get("orphan-repo")
	if got.Status != state.StatusInterrupted {
		t.Errorf("status = %q, want it left at %q so it can be retried",
			got.Status, state.StatusInterrupted)
	}
	// The failure reason replaces the interruption note, and the claim's
	// cleared ExitedAt is restored so the track doesn't look live.
	if got.ErrorMsg == "" {
		t.Error("ErrorMsg empty; want the restore failure recorded")
	}
	if got.ExitedAt == nil {
		t.Error("ExitedAt nil after a released claim; the track would read as still running")
	}
}

// A worktree-less track (doc / ask / plan) owns no worktree to restore,
// so reopening it must skip straight to the spawn — it must not try to
// check out a branch it never had. The spawn then fails against the
// test's nonexistent tmux session, which is the assertion that pins the
// skip: the track comes back interrupted (retryable), not errored, and
// the failure is about spawning rather than about a worktree.
func TestReopenWorktreelessTrackSkipsWorktreeRestore(t *testing.T) {
	srv := newQuietServer(t)
	docDir := t.TempDir()
	tr := state.Track{
		ID:        "doc-track",
		Status:    state.StatusInterrupted,
		SessionID: "77777777-7777-7777-7777-777777777777",
		Kind:      state.KindDoc,
		Slug:      "q3-deck",
		DocPath:   filepath.Join(docDir, "q3-deck.md"),
		// A doc track holds the primary checkout path, not a tracks
		// worktree, and carries no branch.
		Repos: []state.TrackRepo{{Name: "demo", Path: docDir}},
	}
	if err := srv.store.Put(tr); err != nil {
		t.Fatalf("put: %v", err)
	}

	_, err := srv.resumeTrackSession(context.Background(), tr, func(string) {})
	if err == nil {
		t.Fatal("resume succeeded; expected the spawn to fail against the test session")
	}
	if strings.Contains(err.Error(), "worktree") || strings.Contains(err.Error(), "branch") {
		t.Errorf("error mentions worktree/branch (%v); a worktree-less track must skip that step", err)
	}
	if !strings.Contains(err.Error(), "spawn claude") {
		t.Errorf("error = %v, want it to come from the spawn step", err)
	}
	if got, _ := srv.store.Get("doc-track"); got.Status != state.StatusInterrupted {
		t.Errorf("status = %q, want it left at %q so it stays retryable",
			got.Status, state.StatusInterrupted)
	}
}

// Releasing a claim restores the exit bookkeeping the claim cleared. A
// failed resume must not leave a long-finished track looking like it
// exited just now (which inflates its reported runtime) or drop its exit
// code.
func TestResumeTrackSessionReleaseRestoresExitFields(t *testing.T) {
	srv := newQuietServer(t)
	exited := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	code := 0
	tr := state.Track{
		ID:        "finished",
		Status:    state.StatusDone,
		SessionID: "66666666-6666-6666-6666-666666666666",
		Kind:      state.KindWork,
		Branch:    "feat/gone",
		CreatedAt: exited.Add(-time.Hour),
		ExitedAt:  &exited,
		ExitCode:  &code,
		// Unknown repo with a missing path: the restore fails, so the claim
		// is released without anything being spawned.
		Repos: []state.TrackRepo{{Name: "ghost", Path: "/nonexistent/ghost", Branch: "feat/gone"}},
	}
	if err := srv.store.Put(tr); err != nil {
		t.Fatalf("put: %v", err)
	}

	if _, err := srv.resumeTrackSession(context.Background(), tr, func(string) {}); err == nil {
		t.Fatal("resume succeeded; want the worktree restore to fail")
	}

	got, _ := srv.store.Get("finished")
	if got.Status != state.StatusDone {
		t.Errorf("status = %q, want it back at %q", got.Status, state.StatusDone)
	}
	if got.ExitedAt == nil || !got.ExitedAt.Equal(exited) {
		t.Errorf("ExitedAt = %v, want the original %v", got.ExitedAt, exited)
	}
	if got.ExitCode == nil || *got.ExitCode != code {
		t.Errorf("ExitCode = %v, want the original %d", got.ExitCode, code)
	}
}

// A track is claimed atomically before any work starts, so a second
// resume of the same track loses the race instead of spawning a
// duplicate `claude --resume` on one session UUID.
func TestResumeTrackSessionClaimIsExclusive(t *testing.T) {
	srv := newQuietServer(t)
	tr := state.Track{
		ID:        "claimed",
		Status:    state.StatusInterrupted,
		SessionID: "55555555-5555-5555-5555-555555555555",
		Kind:      state.KindWork,
		Branch:    "feat/gone",
		Repos:     []state.TrackRepo{{Name: "ghost", Path: "/nonexistent/ghost", Branch: "feat/gone"}},
	}
	if err := srv.store.Put(tr); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Simulate the first claim having been taken: the track is Pending, so
	// it is no longer terminal.
	if _, _, err := srv.store.Update(tr.ID, func(cur *state.Track) bool {
		cur.Status = state.StatusPending
		return true
	}); err != nil {
		t.Fatalf("pre-claim: %v", err)
	}

	if _, err := srv.resumeTrackSession(context.Background(), tr, func(string) {}); err == nil {
		t.Fatal("second resume succeeded; want it to lose the claim race")
	}
	// The first claim must survive the loser's failure path.
	if got, _ := srv.store.Get(tr.ID); got.Status != state.StatusPending {
		t.Errorf("status = %q, want the existing claim (%q) untouched",
			got.Status, state.StatusPending)
	}
}
