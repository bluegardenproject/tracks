package tracks

import (
	"context"
	"errors"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestAddRepo(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	made, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id := made.Track.ID
	f.worktrees.renamed = map[string]string{"api": "fix/rates"}

	var steps []string
	got, err := f.svc.AddRepo(ctx, id, "WEB", func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "web" || got.Branch != "fix/rates" || got.Worktree != "/wt/"+id+"/web" || got.Base != "develop" || got.RepoID == 0 {
		t.Errorf("added %+v; want web on the branch api is on now", got)
	}
	if len(steps) == 0 {
		t.Error("AddRepo told no progress")
	}
	saved, _ := f.store.Track(ctx, id)
	if len(saved.Repos) != 2 || saved.Repos[1] != got {
		t.Errorf("saved repos %+v", saved.Repos)
	}

	for name, want := range map[string]string{
		"web":  "web is already in " + made.Track.Name + ".",
		"docs": "No repo named docs on the Repositories tab. Repos: api, web.",
	} {
		if _, err := f.svc.AddRepo(ctx, id, name, func(string) {}); err == nil || err.Error() != want {
			t.Errorf("adding %s: %v, want %q", name, err, want)
		}
	}
}

func TestAddRepoRefusesTracksWithoutWorktrees(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	for kind, want := range map[track.Kind]string{
		track.Ask:  " has no worktrees: promote it first.",
		track.Plan: " has no worktrees: promote it first.",
	} {
		made, err := f.svc.Create(ctx, Request{Kind: kind, Repos: []string{"api"}, Prompt: "Why"}, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.AddRepo(ctx, made.Track.ID, "web", func(string) {}); err == nil || err.Error() != made.Track.Name+want {
			t.Errorf("%s track: %v", kind, err)
		}
	}
	made, err := f.svc.Create(ctx, Request{Kind: track.Review, Repos: []string{"api"}, Prompt: "Review", ReviewRef: "feat/x"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.AddRepo(ctx, made.Track.ID, "web", func(string) {}); err == nil || err.Error() != "Only Work tracks can add repos." {
		t.Errorf("review track: %v", err)
	}
}

func TestAddRepoRemovesTheWorktreeWhenSavingFails(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	made, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	f.store.fail = true
	if _, err := f.svc.AddRepo(ctx, made.Track.ID, "web", func(string) {}); !errors.Is(err, errStep) {
		t.Fatalf("err = %v, want the store's", err)
	}
	if len(f.worktrees.removed) != 1 || f.worktrees.removed[0] != made.Track.ID {
		t.Errorf("removed %v; want the new worktree removed", f.worktrees.removed)
	}
	f.store.fail = false
	if saved, _ := f.store.Track(ctx, made.Track.ID); len(saved.Repos) != 1 {
		t.Errorf("saved repos %+v", saved.Repos)
	}
}
