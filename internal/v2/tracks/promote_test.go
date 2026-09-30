package tracks

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestPromoteAnOpenPlanTrack(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	made, err := f.svc.Create(ctx, Request{Kind: track.Plan, Repos: []string{"api", "web"}, Prompt: "Plan the cache"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	f.engine.replies = map[string]string{made.Track.Session: "1. Warm the cache"}

	var steps []string
	got, err := f.svc.Promote(ctx, made.Track.ID, func(s string) { steps = append(steps, s) })
	if err != nil {
		t.Fatal(err)
	}
	if got.Track.ID != made.Track.ID || got.Track.Name != made.Track.Name || got.Track.Kind != track.Work || got.Window.ID != made.Window.ID {
		t.Errorf("promoted %+v in %+v; want the same track, now Work, in its window", got.Track, got.Window)
	}
	if len(f.windows.respawned) != 1 || len(f.windows.opened) != 1 || f.windows.respawned[0].Kind != "work" {
		t.Errorf("respawned %+v, opened %d; want the window's agent restarted", f.windows.respawned, len(f.windows.opened))
	}
	branch := track.Branch(made.Track.ID)
	want := "Plan the cache\n\n---\nThe read-only investigation/plan phase is complete. A worktree has been created on branch `" +
		branch + "` — implement the change here.\n\nThis is how you ended that phase:\n\n1. Warm the cache"
	if f.engine.spec.Track.Prompt != want || f.engine.spec.Track.Kind != track.Work {
		t.Errorf("the agent starts on a %s track with %q", f.engine.spec.Track.Kind, f.engine.spec.Track.Prompt)
	}
	if len(f.engine.spec.DraftPRs) != 1 || f.engine.spec.DraftPRs[0] != "api" {
		t.Errorf("draft PRs %v, want api's", f.engine.spec.DraftPRs)
	}
	if len(steps) == 0 {
		t.Error("Promote told no progress")
	}

	saved, err := f.store.Track(ctx, made.Track.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Kind != track.Work || saved.Prompt != want || !saved.Open() || len(saved.Repos) != 2 {
		t.Fatalf("saved %+v", saved)
	}
	for _, r := range saved.Repos {
		if r.Worktree != "/wt/"+made.Track.ID+"/"+r.Name || r.Branch != branch {
			t.Errorf("saved repo %+v, want its worktree", r)
		}
	}

	if _, err := f.svc.Promote(ctx, made.Track.ID, func(string) {}); err == nil || err.Error() != "Only Ask and Plan tracks can be promoted." {
		t.Errorf("promoting it again: %v", err)
	}
}

func TestPromoteAnEndedAskTrack(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	made, err := f.svc.Create(ctx, Request{Kind: track.Ask, Repos: []string{"web"}, Prompt: "Why is it slow?"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.End(ctx, made.Track.ID); err != nil {
		t.Fatal(err)
	}
	got, err := f.svc.Promote(ctx, made.Track.ID, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.windows.respawned) != 0 || len(f.windows.opened) != 2 || f.windows.opened[1].Kind != "work" {
		t.Errorf("respawned %d, opened %+v; want a new window", len(f.windows.respawned), f.windows.opened)
	}
	if !strings.HasSuffix(f.engine.spec.Track.Prompt, "implement the change here.") {
		t.Errorf("without a reply the prompt is v1's: %q", f.engine.spec.Track.Prompt)
	}
	if saved, _ := f.store.Track(ctx, got.Track.ID); !saved.Open() || saved.Kind != track.Work {
		t.Errorf("saved %+v; want an open Work track", saved)
	}
}

func TestPromoteRefuses(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	for _, c := range []struct {
		req  Request
		want string
	}{
		{Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix"}, "Only Ask and Plan tracks can be promoted."},
		{Request{Kind: track.Ask, Prompt: "Why?"}, " has no repos to promote."},
	} {
		made, err := f.svc.Create(ctx, c.req, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.svc.Promote(ctx, made.Track.ID, func(string) {}); err == nil || !strings.HasSuffix(err.Error(), c.want) {
			t.Errorf("promoting a %s track: %v, want %q", c.req.Kind, err, c.want)
		}
	}
	if _, err := f.svc.Promote(ctx, "gone", func(string) {}); err == nil {
		t.Error("promoted a track that isn't there")
	}
}

func TestPromoteUndoes(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	made, err := f.svc.Create(ctx, Request{Kind: track.Plan, Repos: []string{"api"}, Prompt: "Plan it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}

	f.worktrees.fail = true
	if _, err := f.svc.Promote(ctx, made.Track.ID, func(string) {}); !errors.Is(err, errStep) {
		t.Fatalf("worktrees failing: %v", err)
	}
	f.worktrees.fail = false
	f.engine.failCommand = true
	if _, err := f.svc.Promote(ctx, made.Track.ID, func(string) {}); !errors.Is(err, errStep) {
		t.Fatalf("the command failing: %v", err)
	}
	f.engine.failCommand = false
	if len(f.windows.respawned) != 0 || len(f.worktrees.removed) != 1 {
		t.Errorf("respawned %d, removed %v; want the worktrees removed and the agent left running", len(f.windows.respawned), f.worktrees.removed)
	}

	f.store.fail = true
	if _, err := f.svc.Promote(ctx, made.Track.ID, func(string) {}); !errors.Is(err, errStep) {
		t.Fatalf("saving failing: %v", err)
	}
	f.store.fail = false
	if len(f.windows.closed) != 1 || len(f.worktrees.removed) != 2 {
		t.Errorf("closed %v, removed %v; a track that couldn't be saved ends", f.windows.closed, f.worktrees.removed)
	}
	if saved, _ := f.store.Track(ctx, made.Track.ID); saved.Kind != track.Plan || saved.Repos[0].Worktree != "" {
		t.Errorf("saved %+v; want it left a Plan track", saved)
	}
}
