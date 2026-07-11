package daemon

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newPrimaryWithRemote builds a throwaway primary checkout on "develop"
// with an origin remote pointing at itself, enough to create worktrees
// whose HEAD is (or isn't) reachable from a remote ref. Skips if git is
// absent. Returns the primary path.
func newPrimaryWithRemote(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	run := func(wd string, args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = wd
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	run(dir, "git", "init", "-q", "-b", "develop", ".")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(dir, "git", "add", "README.md")
	run(dir, "git", "commit", "-q", "-m", "initial")
	run(dir, "git", "remote", "add", "origin", dir)
	run(dir, "git", "fetch", "origin", "develop", "--quiet")
	return dir
}

// addWorktree creates a worktree for the primary at dst on a new branch
// off origin/develop.
func addWorktree(t *testing.T, primary, dst, branch string) {
	t.Helper()
	cmd := exec.Command("git", "worktree", "add", "-b", branch, dst, "origin/develop")
	cmd.Dir = primary
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v\n%s", err, out)
	}
}

// A clean, fully pushed orphan track dir is removed, not quarantined.
func TestReclaimOrphanTrackDir_CleanIsRemoved(t *testing.T) {
	primary := newPrimaryWithRemote(t)
	worktreeRoot := t.TempDir()
	id := "20260101-000000-clean"
	trackDir := filepath.Join(worktreeRoot, id)
	if err := os.MkdirAll(trackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	addWorktree(t, primary, filepath.Join(trackDir, "repo"), "tracks/clean")

	quarantined, reason, err := ReclaimOrphanTrackDir(context.Background(), worktreeRoot, id)
	if err != nil {
		t.Fatal(err)
	}
	if quarantined {
		t.Fatalf("clean orphan was quarantined (reason %q), want removed", reason)
	}
	if _, err := os.Stat(trackDir); !os.IsNotExist(err) {
		t.Errorf("track dir still present after removal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktreeRoot, QuarantineDirName)); !os.IsNotExist(err) {
		t.Error("quarantine dir was created for a clean orphan")
	}
}

// An orphan with uncommitted work is moved to _recovered, not deleted.
func TestReclaimOrphanTrackDir_DirtyIsQuarantined(t *testing.T) {
	primary := newPrimaryWithRemote(t)
	worktreeRoot := t.TempDir()
	id := "20260101-000000-dirty"
	trackDir := filepath.Join(worktreeRoot, id)
	if err := os.MkdirAll(trackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	wt := filepath.Join(trackDir, "repo")
	addWorktree(t, primary, wt, "tracks/dirty")
	// Uncommitted, untracked work that exists only here.
	if err := os.WriteFile(filepath.Join(wt, "wip.txt"), []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	quarantined, reason, err := ReclaimOrphanTrackDir(context.Background(), worktreeRoot, id)
	if err != nil {
		t.Fatal(err)
	}
	if !quarantined {
		t.Fatal("dirty orphan was not quarantined — work would be lost")
	}
	if reason == "" {
		t.Error("quarantine returned without a reason")
	}
	// Original gone, contents preserved under _recovered/<id>.
	if _, err := os.Stat(trackDir); !os.IsNotExist(err) {
		t.Errorf("original track dir still present after quarantine: %v", err)
	}
	preserved := filepath.Join(worktreeRoot, QuarantineDirName, id, "repo", "wip.txt")
	got, err := os.ReadFile(preserved)
	if err != nil {
		t.Fatalf("preserved file missing: %v", err)
	}
	if string(got) != "precious\n" {
		t.Errorf("preserved content = %q, want %q", got, "precious\n")
	}
}

// A second quarantine of the same id must not clobber the first.
func TestReclaimOrphanTrackDir_QuarantineIsNonClobbering(t *testing.T) {
	primary := newPrimaryWithRemote(t)
	worktreeRoot := t.TempDir()
	id := "20260101-000000-twice"

	makeDirty := func(branch string) {
		trackDir := filepath.Join(worktreeRoot, id)
		if err := os.MkdirAll(trackDir, 0o755); err != nil {
			t.Fatal(err)
		}
		wt := filepath.Join(trackDir, "repo")
		addWorktree(t, primary, wt, branch)
		if err := os.WriteFile(filepath.Join(wt, "wip.txt"), []byte(branch+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// prunePrimary clears git's now-stale worktree admin entries, exactly
	// as the GC callers (recovery.go / cmd/gc.go) do after each pass.
	prunePrimary := func() {
		cmd := exec.Command("git", "worktree", "prune")
		cmd.Dir = primary
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("worktree prune: %v\n%s", err, out)
		}
	}

	makeDirty("tracks/twice-a")
	if q, _, err := ReclaimOrphanTrackDir(context.Background(), worktreeRoot, id); err != nil || !q {
		t.Fatalf("first quarantine: quarantined=%v err=%v", q, err)
	}
	prunePrimary()
	makeDirty("tracks/twice-b")
	if q, _, err := ReclaimOrphanTrackDir(context.Background(), worktreeRoot, id); err != nil || !q {
		t.Fatalf("second quarantine: quarantined=%v err=%v", q, err)
	}

	recovered := filepath.Join(worktreeRoot, QuarantineDirName)
	entries, err := os.ReadDir(recovered)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected 2 preserved copies, got %d: %v", len(entries), names)
	}
}
