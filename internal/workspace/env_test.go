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
