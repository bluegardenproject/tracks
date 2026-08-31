package cursor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func req(t *testing.T, binary string) ReviewRequest {
	t.Helper()
	return ReviewRequest{
		Binary: binary, Workspace: t.TempDir(),
		Diff: "diff --git a/x b/x\n+changed\n", Instructions: "You are a reviewer.",
		LockDir: t.TempDir(), TrackID: "t1",
	}
}

// Guard 1: the environment marker. A reviewer that somehow reaches
// this code must refuse before spawning anything.
func TestReviewRefusesWhenAlreadyInsideAReview(t *testing.T) {
	t.Setenv(ReviewEnvMarker, "1")
	_, err := Review(context.Background(), req(t, "/nonexistent/agent"))
	if !errors.Is(err, ErrAlreadyReviewing) {
		t.Errorf("err = %v, want ErrAlreadyReviewing", err)
	}
}

// Guard 2: the lock. Independent of the environment, because whether
// the agent propagates env to its children cannot be verified from
// here — on an allowlist config it will not even run a command.
func TestReviewRefusesWhileOneIsRunning(t *testing.T) {
	r := req(t, "/nonexistent/agent")
	release, err := acquireReviewLock(r.LockDir, r.TrackID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	_, err = Review(context.Background(), r)
	if !errors.Is(err, ErrAlreadyReviewing) {
		t.Errorf("err = %v, want ErrAlreadyReviewing while a lock is held", err)
	}
}

// The two guards must be independent: with the env marker absent, the
// lock alone still stops a nested review.
func TestLockGuardHoldsWithoutTheEnvMarker(t *testing.T) {
	if v := os.Getenv(ReviewEnvMarker); v != "" {
		t.Fatalf("%s leaked into the test env: %q", ReviewEnvMarker, v)
	}
	r := req(t, "/nonexistent/agent")
	release, _ := acquireReviewLock(r.LockDir, r.TrackID)
	defer release()

	if _, err := Review(context.Background(), r); !errors.Is(err, ErrAlreadyReviewing) {
		t.Errorf("lock alone did not stop a nested review: %v", err)
	}
}

// The lock must not outlive the review, or one review would poison
// every later one for that track.
func TestLockIsReleasedAfterAReview(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'REVIEW OUTCOME: pass'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := req(t, bin)
	if _, err := Review(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	// A second review of the same track must succeed.
	if _, err := Review(context.Background(), r); err != nil {
		t.Errorf("second review blocked — the lock was not released: %v", err)
	}
}

// The reviewer must be spawned with no permission flags: overriding a
// user's approvalMode is not ours to do, and a process that cannot
// execute cannot spawn another reviewer.
func TestReviewPassesNoPermissionFlags(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent")
	// Echo the argv so the test can inspect what was passed.
	script := "#!/bin/sh\nfor a in \"$@\"; do echo \"ARG:$a\"; done\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := Review(context.Background(), req(t, bin))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"--force", "--yolo", "-f", "--trust", "--sandbox"} {
		if strings.Contains(out, "ARG:"+forbidden) {
			t.Errorf("reviewer was spawned with %s, overriding the user's permission config:\n%s", forbidden, out)
		}
	}
	if !strings.Contains(out, "ARG:-p") || !strings.Contains(out, "ARG:--workspace") {
		t.Errorf("expected -p and --workspace, got:\n%s", out)
	}
}

// The marker must reach the child, which is what makes guard 1 work
// one level down.
func TestReviewSetsTheMarkerOnTheChild(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent")
	script := "#!/bin/sh\necho \"MARKER=${TRACKS_REVIEW:-unset}\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := Review(context.Background(), req(t, bin))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "MARKER=1") {
		t.Errorf("child did not receive %s: %q", ReviewEnvMarker, out)
	}
}

func TestReviewNeedsADiff(t *testing.T) {
	r := req(t, "/nonexistent/agent")
	r.Diff = "  "
	if _, err := Review(context.Background(), r); err == nil || errors.Is(err, ErrAlreadyReviewing) {
		t.Errorf("an empty diff should be its own error, got: %v", err)
	}
}

// A lock can outlive its process: closing the track's tmux window
// sends SIGHUP, which runs no defers. Without reclamation that wedges
// the track's reviews permanently — gc doesn't touch this directory
// and there is no --force.
func TestStaleLockIsReclaimed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "review-t1.lock")
	// A pid that cannot be running.
	if err := os.WriteFile(path, []byte("999999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	release, err := acquireReviewLock(dir, "t1")
	if err != nil {
		t.Fatalf("a lock owned by a dead process should be reclaimed: %v", err)
	}
	defer release()

	owner, alive := lockOwner(path)
	if !alive || owner != os.Getpid() {
		t.Errorf("lock not taken over: owner=%d alive=%v, want this process (%d)", owner, alive, os.Getpid())
	}
}

// A live lock must still be obeyed, or the reclamation defeats the
// guard it is meant to make survivable.
func TestLiveLockIsObeyed(t *testing.T) {
	dir := t.TempDir()
	release, err := acquireReviewLock(dir, "t1")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	if _, err := acquireReviewLock(dir, "t1"); !errors.Is(err, ErrAlreadyReviewing) {
		t.Errorf("a lock held by this (live) process was not obeyed: %v", err)
	}
}

// An unreadable or garbage lock body cannot be attributed to a live
// review, and treating it as live would be unrecoverable.
func TestUnparseableLockIsTreatedAsStale(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "review-t1.lock"), []byte("not a pid"), 0o644); err != nil {
		t.Fatal(err)
	}
	release, err := acquireReviewLock(dir, "t1")
	if err != nil {
		t.Errorf("a lock with no readable pid should be reclaimed: %v", err)
		return
	}
	release()
}

// A diff far past Linux's 128 KiB single-argument limit must go
// through whole. CI's Linux runner failed on exactly this when the
// prompt was an argv element; stdin has no such limit, and this test
// fails on Linux if it ever moves back.
func TestLargeDiffSurvivesTheTransport(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent")
	// Report the prompt's length so the test can check the cap held.
	script := "#!/bin/sh\nprintf 'LEN:%s\\n' \"$(wc -c | tr -d ' ')\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := req(t, bin)
	// Comfortably past MAX_ARG_STRLEN (131072) and well under the
	// sanity cap, so nothing is truncated and the whole thing must
	// still arrive.
	r.Diff = strings.Repeat("x", 400_000)

	out, err := Review(context.Background(), r)
	if err != nil {
		t.Fatalf("a 400 KB diff should go through: %v", err)
	}
	got := 0
	if _, err := fmt.Sscanf(strings.TrimSpace(out), "LEN:%d", &got); err != nil {
		t.Fatalf("fake agent did not report a length: %q", out)
	}
	if got < 400_000 {
		t.Errorf("reviewer received %d bytes, want the whole %d-byte diff", got, 400_000)
	}
}

// Beyond the sanity cap it is truncated with a note rather than
// silently sent whole.
func TestPathologicalDiffIsTruncatedWithANote(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\ncat\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := req(t, bin)
	r.Diff = strings.Repeat("y", maxDiffBytes+1000)
	out, err := Review(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "diff truncated at") {
		t.Error("an over-cap diff was sent without a truncation note")
	}
}

// The reviewer definition assumes candor 3 when it is not told, so
// omitting the line silently overrides the level the track was created
// with.
func TestCandorReachesTheReviewer(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent")
	script := "#!/bin/sh\ncat\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	r := req(t, bin)
	r.Candor = 8
	out, err := Review(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Candor level: 8/10") {
		t.Error("the track's candor level did not reach the reviewer")
	}

	// Unset means say nothing and let the definition's default stand.
	r.Candor = 0
	out, err = Review(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Candor level:") {
		t.Error("an unset candor should not invent a level")
	}
}
