package workspace

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCopyEnv(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	api := repo(t, root, "api")
	write := func(dir, rel, body string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(api.Path, ".gitignore", ".env*\n!.env.example\nnode_modules/\n")
	write(api.Path, ".env.example", "tracked")
	run(t, api.Path, "add", ".gitignore", ".env.example")
	run(t, api.Path, "commit", "-q", "-m", "ignore")
	write(api.Path, ".env", "root")
	write(api.Path, "apps/web/.env.local", "web")
	write(api.Path, "apps/web/.envrc", "not an env file")
	write(api.Path, "node_modules/pkg/.env", "skipped")
	worktree := filepath.Join(root, "wt")
	write(worktree, ".env", "kept")

	copied, err := CopyEnv(context.Background(), api.Path, worktree)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(copied, []string{"apps/web/.env.local"}) {
		t.Errorf("copied %v; want only apps/web/.env.local", copied)
	}
	for rel, want := range map[string]string{".env": "kept", "apps/web/.env.local": "web"} {
		if got, _ := os.ReadFile(filepath.Join(worktree, rel)); string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
	for _, rel := range []string{"node_modules/pkg/.env", "apps/web/.envrc", ".env.example"} {
		if _, err := os.Stat(filepath.Join(worktree, rel)); err == nil {
			t.Errorf("%s was copied", rel)
		}
	}
	if info, _ := os.Stat(filepath.Join(worktree, "apps/web/.env.local")); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600 kept", info.Mode().Perm())
	}
}

func TestCopyEnvBlocksSymlinkEscape(t *testing.T) {
	isolate(t)
	root := t.TempDir()
	api := repo(t, root, "api")
	write := func(dir, rel, body string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(api.Path, ".gitignore", ".env*\n")
	run(t, api.Path, "add", ".gitignore")
	run(t, api.Path, "commit", "-q", "-m", "ignore")
	write(api.Path, "apps/web/.env", "SECRET=1")
	write(api.Path, ".env.local", "SECRET=2")

	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(outside, "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(root, "wt")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	// Intermediate directory symlink escaping worktree, plus dangling file symlink.
	if err := os.Symlink(outside, filepath.Join(worktree, "apps")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "escaped.env"), filepath.Join(worktree, ".env.local")); err != nil {
		t.Fatal(err)
	}

	copied, err := CopyEnv(context.Background(), api.Path, worktree)
	if err != nil {
		t.Fatalf("CopyEnv error: %v", err)
	}
	if len(copied) != 0 {
		t.Errorf("copied %v; want nothing written through symlinks", copied)
	}
	for _, p := range []string{filepath.Join(outside, "web/.env"), filepath.Join(outside, "escaped.env")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("CopyEnv escaped worktree and wrote %s", p)
		}
	}
}
