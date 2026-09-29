package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestCleanAndRestoreWork(t *testing.T) {
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
	wt := repos[0].Worktree
	if u, err := w.Unsaved(ctx, tr); err != nil || len(u) != 0 {
		t.Fatalf("a fresh worktree has unsaved work %v, %v", u, err)
	}

	if err := os.WriteFile(filepath.Join(wt, "rates.go"), []byte("package rates\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, wt, "add", "rates.go")
	run(t, wt, "commit", "-q", "-m", "rates")
	for name, text := range map[string]string{"rates.go": "package rates // edited\n", "notes.md": "todo\n"} {
		if err := os.WriteFile(filepath.Join(wt, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	u, err := w.Unsaved(ctx, tr)
	want := "api: 1 changed file, 1 untracked file and 1 commit that exists nowhere else"
	if err != nil || len(u) != 1 || u[0].String() != want {
		t.Fatalf("Unsaved = %v, %v; want %q", u, err, want)
	}

	if err := w.RemoveWorktrees(ctx, tr.ID, tr.Repos); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(w.Root, tr.ID)); !os.IsNotExist(err) {
		t.Errorf("the track's folder is still there: %v", err)
	}
	if out := run(t, api.Path, "branch", "--list", repos[0].Branch); out == "" {
		t.Error("Clean removed the branch")
	}

	var steps []string
	made, err := w.Restore(ctx, tr, func(s string) { steps = append(steps, s) })
	if err != nil || len(made) != 1 {
		t.Fatalf("Restore = %v, %v", made, err)
	}
	if s := subject(t, wt); s != "rates" || run(t, wt, "branch", "--show-current") != repos[0].Branch {
		t.Errorf("restored worktree is at %q on %s", s, run(t, wt, "branch", "--show-current"))
	}
	if len(steps) != 1 || !strings.Contains(steps[0], "Re-creating the worktree for api") {
		t.Errorf("progress = %q", steps)
	}
	if made, err := w.Restore(ctx, tr, noProgress); err != nil || len(made) != 0 {
		t.Errorf("restoring worktrees that exist: %v, %v", made, err)
	}

	if err := w.RemoveWorktrees(ctx, tr.ID, tr.Repos); err != nil {
		t.Fatal(err)
	}
	run(t, api.Path, "branch", "-D", repos[0].Branch)
	if _, err := w.Restore(ctx, tr, noProgress); err == nil || !strings.Contains(err.Error(), "no longer exists in api") {
		t.Errorf("restoring onto a deleted branch: %v", err)
	}
}

func TestCleanAndRestoreReview(t *testing.T) {
	isolate(t)
	ctx := context.Background()
	root := t.TempDir()
	w := &Worktrees{Root: filepath.Join(root, "worktrees")}
	tr := track.Track{ID: "20260928-101500-def456", Kind: track.Review, ReviewRef: "https://github.com/acme/api/pull/7",
		Repos: []track.Repo{repo(t, root, "api")}}
	repos, err := w.Add(ctx, tr, noProgress)
	if err != nil {
		t.Fatal(err)
	}
	tr.Repos = repos
	wt := repos[0].Worktree
	if u, err := w.Unsaved(ctx, tr); err != nil || len(u) != 0 {
		t.Fatalf("the pull request's own commits count as unsaved: %v, %v", u, err)
	}
	run(t, wt, "commit", "-q", "--allow-empty", "-m", "a fix")
	if u, err := w.Unsaved(ctx, tr); err != nil || len(u) != 1 || u[0].Commits != 1 {
		t.Fatalf("a commit on the checkout: Unsaved = %v, %v", u, err)
	}

	if err := w.RemoveWorktrees(ctx, tr.ID, tr.Repos); err != nil {
		t.Fatal(err)
	}
	if made, err := w.Restore(ctx, tr, noProgress); err != nil || len(made) != 1 {
		t.Fatalf("Restore = %v, %v", made, err)
	}
	if s := subject(t, wt); s != "pull request" {
		t.Errorf("restored review is at %q, want the pull request fetched again", s)
	}
}
