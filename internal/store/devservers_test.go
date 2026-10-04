package store

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

func TestDevServersRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	servers := []DevServer{
		{Name: "web", Command: "pnpm dev --port $PORT", Dir: "apps/web", PortMode: PortAssigned, Type: "rspack"},
		{Name: "metro", Command: "pnpm mobile:start", PortMode: PortFixed, Port: 8081},
	}
	added, err := s.AddRepo(ctx, Repo{Name: "shop", Path: "/src/shop", BaseBranch: "main", Setup: "pnpm install", Servers: servers})
	if err != nil {
		t.Fatal(err)
	}
	if added.Setup != "pnpm install" || !slices.Equal(added.Servers, servers) {
		t.Fatalf("added %+v", added)
	}
	if _, err := s.AddRepo(ctx, Repo{Name: "api", Path: "/src/api", BaseBranch: "main"}); err != nil {
		t.Fatal(err)
	}

	added.Setup, added.Servers = "", servers[1:]
	updated, err := s.UpdateRepo(ctx, added)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Setup != "" || !slices.Equal(updated.Servers, servers[1:]) {
		t.Errorf("updated %+v; want no setup and only metro", updated)
	}
	repos, err := s.Repos(ctx)
	if err != nil || len(repos) != 2 || repos[0].Servers != nil || !slices.Equal(repos[1].Servers, servers[1:]) {
		t.Fatalf("Repos = %+v, %v", repos, err)
	}

	if err := s.DeleteRepo(ctx, added.ID); err != nil {
		t.Fatal(err)
	}
	var left int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM dev_servers").Scan(&left); err != nil || left != 0 {
		t.Errorf("dev servers after deleting their repo: %d, %v", left, err)
	}
}

func TestDevServerNamesAreUniquePerRepo(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	twice := []DevServer{{Name: "web", Command: "a", PortMode: PortAssigned}, {Name: "Web", Command: "b", PortMode: PortAssigned}}
	if _, err := s.AddRepo(ctx, Repo{Name: "shop", Path: "/src/shop", BaseBranch: "main", Servers: twice}); err == nil {
		t.Fatal("two servers named web (any case) were stored")
	}
	if repos, _ := s.Repos(ctx); len(repos) != 0 {
		t.Errorf("the repo was stored without its servers: %+v", repos)
	}
}
