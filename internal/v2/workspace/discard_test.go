package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestLostAndDiscard(t *testing.T) {
	isolate(t)
	ctx := context.Background()
	root := t.TempDir()
	api := repo(t, root, "api")
	w := &Worktrees{Root: filepath.Join(root, "worktrees")}
	tr := track.Track{ID: "20260928-101500-abc123", Kind: track.Work, Repos: []track.Repo{api}}
	repos, err := w.Add(ctx, tr, noProgress)
	if err != nil {
		t.Fatal(err)
	}
	tr.Repos = repos
	wt, branch := repos[0].Worktree, repos[0].Branch
	if lost, err := w.Lost(ctx, tr); err != nil || len(lost) != 0 {
		t.Fatalf("a fresh track would lose %v, %v", lost, err)
	}

	if err := os.WriteFile(filepath.Join(wt, "rates.go"), []byte("package rates\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, wt, "add", "rates.go")
	run(t, wt, "commit", "-q", "-m", "rates")
	if err := os.WriteFile(filepath.Join(wt, "notes.md"), []byte("todo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := "api: 1 untracked file and 1 commit that exists nowhere else"
	if lost, err := w.Lost(ctx, tr); err != nil || len(lost) != 1 || lost[0].String() != want {
		t.Fatalf("Lost = %v, %v; want %q", lost, err, want)
	}

	// Removed: the worktree is gone, the branch's commit isn't.
	if err := w.RemoveWorktrees(ctx, tr.ID, tr.Repos); err != nil {
		t.Fatal(err)
	}
	want = "api: 1 commit that exists nowhere else"
	if lost, err := w.Lost(ctx, tr); err != nil || len(lost) != 1 || lost[0].String() != want {
		t.Fatalf("Lost once removed = %v, %v; want %q", lost, err, want)
	}
	run(t, api.Path, "branch", "keep", branch)
	if lost, err := w.Lost(ctx, tr); err != nil || len(lost) != 0 {
		t.Errorf("Lost with the commit on another branch = %v, %v", lost, err)
	}
	run(t, api.Path, "branch", "-D", "keep")
	run(t, api.Path, "push", "-q", "origin", branch)
	if lost, err := w.Lost(ctx, tr); err != nil || len(lost) != 0 {
		t.Errorf("Lost with the branch pushed = %v, %v", lost, err)
	}

	if err := w.Discard(ctx, tr); err != nil {
		t.Fatal(err)
	}
	if out := run(t, api.Path, "branch", "--list", branch); out != "" {
		t.Errorf("Discard left the branch: %q", out)
	}
	if out := run(t, api.Path, "branch", "-r", "--list", "origin/"+branch); out == "" {
		t.Error("Discard deleted the pushed branch")
	}
	if lost, err := w.Lost(ctx, tr); err != nil || len(lost) != 0 {
		t.Errorf("Lost once discarded = %v, %v", lost, err)
	}
	if err := w.Discard(ctx, tr); err != nil {
		t.Errorf("discarding again: %v", err)
	}
}

func TestDiscardWithWorktrees(t *testing.T) {
	isolate(t)
	ctx := context.Background()
	root := t.TempDir()
	api := repo(t, root, "api")
	w := &Worktrees{Root: filepath.Join(root, "worktrees")}
	tr := track.Track{ID: "20260928-101500-def456", Kind: track.Work, Repos: []track.Repo{api}}
	repos, err := w.Add(ctx, tr, noProgress)
	if err != nil {
		t.Fatal(err)
	}
	tr.Repos = repos
	run(t, repos[0].Worktree, "branch", "-m", "fix/rates")

	if err := w.Discard(ctx, tr); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.Root, tr.ID)); !os.IsNotExist(err) {
		t.Errorf("the track's folder is still there: %v", err)
	}
	if out := run(t, api.Path, "branch", "--list", "fix/rates", repos[0].Branch); out != "" {
		t.Errorf("Discard left the renamed branch: %q", out)
	}

	// The base branch, checked out in the primary checkout, stays.
	tr.Repos[0].Branch, tr.Repos[0].Worktree = "main", ""
	if err := w.Discard(ctx, tr); err != nil {
		t.Fatal(err)
	}
	if out := run(t, api.Path, "branch", "--list", "main"); out == "" {
		t.Error("Discard deleted the base branch")
	}
}
