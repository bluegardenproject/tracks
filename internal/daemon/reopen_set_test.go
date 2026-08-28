package daemon

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/state"
)

// The reopen set is "what the user had open", not "what was running".
// A pr-merged track kept open for the next round of work on the same
// topic comes back with the interrupted ones; a track that was closed
// does not, whatever its status.
func TestReopenTargetsEveryOpenTrackOldestFirst(t *testing.T) {
	srv := newQuietServer(t)
	base := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	const sid = "44444444-4444-4444-4444-444444444444"
	for _, tr := range []state.Track{
		{ID: "interrupted", Status: state.StatusInterrupted, SessionID: sid,
			WindowOpen: true, CreatedAt: base.Add(2 * time.Hour)},
		{ID: "merged-kept-open", Status: state.StatusPRMerged, SessionID: sid,
			WindowOpen: true, CreatedAt: base},
		// Every *selected* fixture needs a distinct CreatedAt: Store.All
		// sorts with sort.Slice over a map range, so a tie between two
		// tracks that both land in the result orders them at random.
		{ID: "done-kept-open", Status: state.StatusDone, SessionID: sid,
			WindowOpen: true, CreatedAt: base.Add(time.Hour)},
		// Closed: `tracks done` killed the window and cleared the flag.
		{ID: "done-and-closed", Status: state.StatusDone, SessionID: sid, CreatedAt: base},
		// Predates WindowOpen, so the flag is absent; interrupted tracks
		// keep the behaviour they had.
		{ID: "legacy-interrupted", Status: state.StatusInterrupted, SessionID: sid,
			CreatedAt: base.Add(3 * time.Hour)},
		// No session to resume, but not dropped silently: handleReopen
		// reports it as a per-track failure saying why.
		{ID: "no-session", Status: state.StatusDone, WindowOpen: true,
			CreatedAt: base.Add(30 * time.Minute)},
		{ID: "draft", Status: state.StatusDraft, SessionID: sid, WindowOpen: true, CreatedAt: base},
		// Live: it has a window to attach to, not one to rebuild.
		{ID: "running", Status: state.StatusRunning, SessionID: sid,
			WindowOpen: true, CreatedAt: base},
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
	want := []string{"merged-kept-open", "no-session", "done-kept-open", "interrupted", "legacy-interrupted"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("targets = %v, want %v (oldest first)", got, want)
	}
}

// Naming a track that was closed is a mistake worth an error: `tracks
// resume` is the command for that, and silently skipping it would look
// like the reopen worked.
func TestReopenTargetsRejectsAClosedTrack(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:        "closed",
		Status:    state.StatusPRMerged,
		SessionID: "55555555-5555-5555-5555-555555555555",
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	_, err := srv.reopenTargets([]string{"closed"})
	if err == nil {
		t.Fatal("closed track accepted; want an error")
	}
	if !strings.Contains(err.Error(), "tracks resume") {
		t.Errorf("error = %q, want it to name the command that does work", err)
	}
}

// Closing a track is what takes it out of the reopen set — including a
// track that had already finished on its own, whose status doesn't
// change on the way out and so can't carry the fact.
func TestEndTrackClearsTheOpenFlag(t *testing.T) {
	srv := newQuietServer(t)
	exited := time.Date(2026, 8, 27, 19, 3, 0, 0, time.UTC)
	if err := srv.store.Put(state.Track{
		ID:         "closed-by-hand",
		Kind:       state.KindAsk,
		Status:     state.StatusPRMerged,
		SessionID:  "66666666-6666-6666-6666-666666666666",
		WindowOpen: true,
		ExitedAt:   &exited,
		PRs:        []state.PRRef{{URL: "https://example.test/pr/1", State: "MERGED"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	resp := srv.endTrack(context.Background(),
		mustParams(t, DoneParams{ID: "closed-by-hand"}), false, func(string) {})
	if !resp.Ok {
		t.Fatalf("endTrack failed: %s", resp.Error)
	}

	got, _ := srv.store.Get("closed-by-hand")
	if got.WindowOpen {
		t.Error("WindowOpen still set; the track would be reopened after the user closed it")
	}
	if got.ShouldReopen() {
		t.Error("a closed track is still in the reopen set")
	}
	if got.Status != state.StatusPRMerged {
		t.Errorf("status = %q, want it left at %q", got.Status, state.StatusPRMerged)
	}
}

// A daemon crash can leave a track whose Claude is somehow still
// running. reconcileOnStartup marks it errored rather than interrupted —
// it can't re-supervise that process — and it must also leave the reopen
// set: reopening kills the window and spawns `claude --resume` on a
// session another process is still writing to.
func TestReconcileTakesAnOrphanedLiveTrackOutOfTheReopenSet(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:         "orphan-alive",
		Status:     state.StatusRunning,
		SessionID:  "99999999-9999-9999-9999-999999999999",
		WindowOpen: true,
		PID:        os.Getpid(), // alive by construction
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	srv.reconcileOnStartup(context.Background())

	got, _ := srv.store.Get("orphan-alive")
	if got.Status != state.StatusErrored {
		t.Fatalf("status = %q, want %q", got.Status, state.StatusErrored)
	}
	if got.WindowOpen || got.ShouldReopen() {
		t.Error("still in the reopen set; reopening it would fork a live session")
	}
}

// "Clear completed" must not delete the record a reopen needs. A
// pr-merged track kept open is Completed but is coming back on the next
// start, and the record is the only handle it has.
func TestPruneCompletedKeepsAKeptOpenTrack(t *testing.T) {
	srv := newQuietServer(t)
	const sid = "12345678-1234-1234-1234-123456789abc"
	for _, tr := range []state.Track{
		{ID: "kept-open", Status: state.StatusPRMerged, SessionID: sid, WindowOpen: true},
		{ID: "closed", Status: state.StatusPRMerged, SessionID: sid},
	} {
		if err := srv.store.Put(tr); err != nil {
			t.Fatalf("put %s: %v", tr.ID, err)
		}
	}

	srv.handlePruneCompleted()

	if _, ok := srv.store.Get("kept-open"); !ok {
		t.Error("a track the user still has open was pruned; its reopen handle is gone")
	}
	if _, ok := srv.store.Get("closed"); ok {
		t.Error("a closed pr-merged track survived the prune")
	}
}

// End to end through the daemon method: a pr-merged track the user kept
// open is reported as a reopen target, which is the whole point of the
// change. The spawn then fails against the test's nonexistent session,
// so the assertion is on what was attempted, not on the window.
func TestReopenAttemptsAPRMergedTrackTheUserKeptOpen(t *testing.T) {
	srv := newQuietServer(t)
	if err := srv.store.Put(state.Track{
		ID:         "kept-open",
		Kind:       state.KindWork,
		Status:     state.StatusPRMerged,
		SessionID:  "88888888-8888-8888-8888-888888888888",
		Slug:       "same-topic",
		WindowOpen: true,
		Repos:      []state.TrackRepo{{Name: "demo", Path: t.TempDir(), Branch: "feat/x"}},
	}); err != nil {
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
	if len(res.Reopened)+len(res.Failed) != 1 {
		t.Fatalf("reopened %+v / failed %+v, want the track to have been attempted",
			res.Reopened, res.Failed)
	}
	// "spawn " rather than a provider name: the label is the track's
	// provider, and this test doesn't care which one.
	if len(res.Failed) == 1 && !strings.Contains(res.Failed[0].Error, "spawn ") {
		t.Errorf("failure = %q, want it to come from the spawn step", res.Failed[0].Error)
	}
	// The claim was released back to where it came from.
	if got, _ := srv.store.Get("kept-open"); got.Status != state.StatusPRMerged {
		t.Errorf("status = %q, want it back at %q", got.Status, state.StatusPRMerged)
	}
}
