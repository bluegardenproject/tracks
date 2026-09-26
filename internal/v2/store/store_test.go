package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func open(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestReposRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	added, err := s.AddRepo(ctx, Repo{Name: "shop-web", Path: "/src/shop-web", BaseBranch: "main", DraftPRs: true})
	if err != nil {
		t.Fatal(err)
	}
	if added.ID == 0 || added.CreatedAt.IsZero() || !added.DraftPRs || added.BaseBranch != "main" {
		t.Fatalf("added %+v", added)
	}
	added.Name, added.BaseBranch, added.DraftPRs = "web", "develop", false
	updated, err := s.UpdateRepo(ctx, added)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "web" || updated.BaseBranch != "develop" || updated.DraftPRs || updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Errorf("updated %+v", updated)
	}
	if _, err := s.AddRepo(ctx, Repo{Name: "api", Path: "/src/api", BaseBranch: "main"}); err != nil {
		t.Fatal(err)
	}
	repos, err := s.Repos(ctx)
	if err != nil || len(repos) != 2 || repos[0].Name != "api" || repos[1].Name != "web" {
		t.Fatalf("Repos = %+v, %v; want api and web, by name", repos, err)
	}
	if err := s.DeleteRepo(ctx, updated.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Repo(ctx, updated.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted repo: %v, want ErrNotFound", err)
	}
	if err := s.DeleteRepo(ctx, updated.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting twice: %v, want ErrNotFound", err)
	}
}

func TestReposAreUnique(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	first, err := s.AddRepo(ctx, Repo{Name: "shop", Path: "/src/shop", BaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddRepo(ctx, Repo{Name: "Shop", Path: "/src/other", BaseBranch: "main"}); !errors.Is(err, ErrNameTaken) {
		t.Errorf("same name in another case: %v, want ErrNameTaken", err)
	}
	second, err := s.AddRepo(ctx, Repo{Name: "other", Path: "/src/other", BaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	second.Path = first.Path
	if _, err := s.UpdateRepo(ctx, second); !errors.Is(err, ErrPathTaken) {
		t.Errorf("moving onto another repo's path: %v, want ErrPathTaken", err)
	}
}

func TestOpenRefusesNewerDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracks.db")
	open(t, path).Close()
	setVersion(t, path, 99)
	if _, err := Open(context.Background(), path); !errors.Is(err, ErrTooNew) {
		t.Errorf("Open of a newer database: %v, want ErrTooNew", err)
	}
}

func TestMigratingBacksUpFirst(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tracks.db")
	db := rawDB(t, path)
	first := []string{"CREATE TABLE a (v INTEGER) STRICT; INSERT INTO a VALUES (1);"}
	if err := migrate(ctx, db, path, first); err != nil {
		t.Fatal(err)
	}
	if backups, _ := filepath.Glob(path + ".*.bak"); len(backups) != 0 {
		t.Fatalf("a new database got backups %v", backups)
	}
	if err := migrate(ctx, db, path, append(first, "CREATE TABLE b (v INTEGER) STRICT;")); err != nil {
		t.Fatal(err)
	}
	backups, _ := filepath.Glob(path + ".*.bak")
	if len(backups) != 1 {
		t.Fatalf("backups %v, want one", backups)
	}
	var v, version int
	b := rawDB(t, backups[0])
	if err := b.QueryRow("SELECT v FROM a").Scan(&v); err != nil || v != 1 {
		t.Errorf("backup holds %d, %v; want the row from before", v, err)
	}
	if err := b.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Errorf("backup is at version %d, %v; want 1", version, err)
	}
}

func setVersion(t *testing.T, path string, v int) {
	t.Helper()
	if _, err := rawDB(t, path).Exec(fmt.Sprintf("PRAGMA user_version = %d", v)); err != nil {
		t.Fatal(err)
	}
}

func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestConnectionSettings(t *testing.T) {
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	for pragma, want := range map[string]string{
		"journal_mode": "wal", "foreign_keys": "1", "synchronous": "1", "busy_timeout": "5000",
	} {
		var got string
		if err := s.db.QueryRow("PRAGMA " + pragma).Scan(&got); err != nil || got != want {
			t.Errorf("PRAGMA %s = %q, %v; want %q", pragma, got, err, want)
		}
	}
}

func TestConcurrentWritersWait(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tracks.db")
	a, b := open(t, path), open(t, path)
	errs := make(chan error, 2)
	for i, s := range []*Store{a, b} {
		go func() {
			for j := range 50 {
				name := fmt.Sprintf("repo-%d-%d", i, j)
				if _, err := s.AddRepo(ctx, Repo{Name: name, Path: "/src/" + name, BaseBranch: "main"}); err != nil {
					errs <- err
					return
				}
			}
			errs <- nil
		}()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("a writer failed: %v", err)
		}
	}
	if repos, err := a.Repos(ctx); err != nil || len(repos) != 100 {
		t.Errorf("%d repos, %v; want 100", len(repos), err)
	}
}
