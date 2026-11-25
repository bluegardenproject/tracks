package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
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

func TestTamperedWorktreeDotGitRefused(t *testing.T) {
	isolate(t)
	ctx := context.Background()
	root := t.TempDir()
	api := repo(t, root, "api")
	w := &Worktrees{Root: filepath.Join(root, "worktrees")}
	tr := track.Track{ID: "20260928-101500-sec123", Kind: track.Work, Repos: []track.Repo{api}}
	repos, err := w.Add(ctx, tr, noProgress)
	if err != nil {
		t.Fatal(err)
	}
	tr.Repos = repos
	wt := repos[0].Worktree

	// Replace the worktree's .git file with a crafted .git/ directory
	// carrying a malicious core.fsmonitor hook.
	dotGit := filepath.Join(wt, ".git")
	if err := os.Remove(dotGit); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dotGit, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(root, "pwned")
	cfg := "[core]\n\trepositoryformatversion = 0\n\tfsmonitor = touch " + sentinel + "\n"
	if err := os.WriteFile(filepath.Join(dotGit, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotGit, "HEAD"), []byte("ref: refs/heads/evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Lost(ctx, tr); err == nil {
		t.Error("Lost accepted a tampered .git directory")
	}
	got := w.Branches(ctx, tr)
	if got[0].Branch != tr.Repos[0].Branch {
		t.Errorf("Branches read tampered branch %q, want recorded %q", got[0].Branch, tr.Repos[0].Branch)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatalf("fsmonitor from tampered worktree .git executed! sentinel err=%v", err)
	}
}

func TestRelativeWorktreePathsAndSeparateGitDir(t *testing.T) {
	isolate(t)
	ctx := context.Background()
	root := t.TempDir()
	api := repo(t, root, "api")
	run(t, api.Path, "config", "worktree.useRelativePaths", "true")

	w := &Worktrees{Root: filepath.Join(root, "worktrees")}
	// Symlinked worktrees root at a different depth than the physical target.
	realRoot := filepath.Join(root, "deep", "nested", "worktrees")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, w.Root); err != nil {
		t.Fatal(err)
	}
	tr := track.Track{ID: "20260928-101500-rel123", Kind: track.Work, Repos: []track.Repo{api}}
	repos, err := w.Add(ctx, tr, noProgress)
	if err != nil {
		t.Fatal(err)
	}
	tr.Repos = repos
	if err := os.WriteFile(filepath.Join(repos[0].Worktree, "note.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lost, err := w.Lost(ctx, tr)
	if err != nil || len(lost) != 1 || lost[0].Untracked != 1 {
		t.Fatalf("relative worktree Lost = %v, %v; want 1 untracked", lost, err)
	}

	// Separate git dir primary repo (.git is a gitfile).
	sepGit := filepath.Join(root, "sep.git")
	sepWork := filepath.Join(root, "sep")
	run(t, root, "clone", "-q", "--separate-git-dir="+sepGit, filepath.Join(root, "api.git"), sepWork)
	sepRepo := track.Repo{Name: "sep", Path: sepWork, Base: "main"}
	tr2 := track.Track{ID: "20260928-101500-sep123", Kind: track.Work, Repos: []track.Repo{sepRepo}}
	repos2, err := w.Add(ctx, tr2, noProgress)
	if err != nil {
		t.Fatal(err)
	}
	tr2.Repos = repos2
	if lost, err := w.Lost(ctx, tr2); err != nil || len(lost) != 0 {
		t.Fatalf("separate-git-dir Lost = %v, %v; want clean", lost, err)
	}

	// Nested fake admin directory under .git/worktrees/<name>/evil must be refused.
	gitDir, err := trustedWorktreeGitDir(sepWork, repos2[0].Worktree)
	if err != nil {
		t.Fatalf("trustedWorktreeGitDir: %v", err)
	}
	nested := filepath.Join(gitDir, "evil")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "gitdir"), []byte(filepath.Join(repos2[0].Worktree, ".git")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "commondir"), []byte("../../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repos2[0].Worktree, ".git"), []byte("gitdir: "+nested+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Lost(ctx, tr2); err == nil {
		t.Error("Lost accepted nested admin dir under .git/worktrees/<name>/evil")
	}
}
