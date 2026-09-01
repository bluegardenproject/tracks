package git

import (
	"context"
	"strings"
	"testing"
)

// recordingRunner captures the argv of every Run call so a test can
// assert on the revision range a method asked git for.
type recordingRunner struct {
	argv [][]string
	out  string
}

func (r *recordingRunner) Run(ctx context.Context, args ...string) (string, string, error) {
	r.argv = append(r.argv, args)
	return r.out, "", nil
}

// The committed half must use three dots. Two dots is a plain
// two-endpoint diff, so every commit that landed on the base after
// this worktree branched shows up as this branch's work, inverted —
// somebody else's new file reads as a deletion.
func TestChangedFilesDiffsFromTheMergeBase(t *testing.T) {
	r := &recordingRunner{}
	c := &WorktreeClient{Path: "/x", Runner: r}

	if _, err := c.ChangedFiles(context.Background(), "origin/develop"); err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}

	if len(r.argv) != 2 {
		t.Fatalf("expected 2 git calls (committed + uncommitted), got %d: %v", len(r.argv), r.argv)
	}
	committed := strings.Join(r.argv[0], " ")
	if !strings.Contains(committed, "origin/develop...HEAD") {
		t.Errorf("committed half must diff from the merge-base, got: %s", committed)
	}
	// The uncommitted half is a working-tree diff against HEAD, which
	// takes no range at all.
	if uncommitted := strings.Join(r.argv[1], " "); !strings.HasSuffix(uncommitted, "HEAD") ||
		strings.Contains(uncommitted, "..") {
		t.Errorf("uncommitted half should be a bare `diff HEAD`, got: %s", uncommitted)
	}
}

// CommitLog's two dots are correct — `log A..B` is already the set
// difference. Three dots there would be the symmetric difference and
// would pull the base's own commits into the track's log.
func TestCommitLogUsesTheSetDifference(t *testing.T) {
	r := &recordingRunner{}
	c := &WorktreeClient{Path: "/x", Runner: r}

	if _, err := c.CommitLog(context.Background(), "origin/develop"); err != nil {
		t.Fatalf("CommitLog: %v", err)
	}

	got := strings.Join(r.argv[0], " ")
	if !strings.Contains(got, "origin/develop..HEAD") || strings.Contains(got, "...HEAD") {
		t.Errorf("expected two-dot `log origin/develop..HEAD`, got: %s", got)
	}
}
