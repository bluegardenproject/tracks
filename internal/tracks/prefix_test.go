package tracks

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
)

func TestCreateShowsTheReposPrefixes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	repos, err := f.store.Repos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range repos {
		r.Prefix = map[string]string{"api": "[API]", "web": "[WEB]"}[r.Name]
		if _, err := f.store.UpdateRepo(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	docs := store.Repo{Name: "docs", Path: filepath.Join(t.TempDir(), "docs"), BaseBranch: "main", Prefix: "[API]"}
	if err := os.Mkdir(docs.Path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.AddRepo(ctx, docs); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name  string
		req   Request
		title string
	}{
		{"one repo, named", Request{Kind: track.Work, Name: "Fix login", Repos: []string{"web"}, Prompt: "Fix"}, "[WEB] Fix login"},
		{"one repo, unnamed", Request{Kind: track.Work, Repos: []string{"web"}, Prompt: "Fix the login"}, "[WEB] fix-the-login"},
		{"two repos", Request{Kind: track.Work, Name: "Fix", Repos: []string{"web", "api"}, Prompt: "Fix"}, "[WEB] [API] Fix"},
		{"two repos, one prefix", Request{Kind: track.Work, Name: "Fix", Repos: []string{"api", "docs"}, Prompt: "Fix"}, "[API] Fix"},
		{"three repos", Request{Kind: track.Work, Name: "Fix", Repos: []string{"web", "api", "docs"}, Prompt: "Fix"}, "Fix"},
		{"its own title", Request{Kind: track.Review, Title: "[Fast Review] Fix", Repos: []string{"web"},
			ReviewRef: "https://github.com/acme/web/pull/7"}, "[Fast Review] Fix"},
	} {
		got, err := f.svc.Create(ctx, c.req, func(string) {})
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got.Track.Title != c.title {
			t.Errorf("%s: title %q, want %q", c.name, got.Track.Title, c.title)
		}
	}
}
