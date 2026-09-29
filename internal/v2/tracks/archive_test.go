package tracks

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/workspace"
)

func TestArchive(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	work, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id := work.Track.ID
	if _, err := f.svc.Archive(ctx, id, true); err == nil || !strings.Contains(err.Error(), "End fix-it before") {
		t.Errorf("archiving an open track: %v", err)
	}
	if err := f.svc.End(ctx, id); err != nil {
		t.Fatal(err)
	}

	f.worktrees.lost = []workspace.Unsaved{{Repo: "api", Commits: 2}}
	if lost, err := f.svc.Archive(ctx, id, false); err != nil || len(lost) != 1 {
		t.Fatalf("Archive = %v, %v; want the work that would be lost", lost, err)
	}
	if saved, _ := f.store.Track(ctx, id); saved.Archived() || saved.Cleaned() || len(f.worktrees.discarded) != 0 {
		t.Fatalf("Archive changed %+v with work that would be lost", saved.State)
	}
	f.worktrees.renamed = map[string]string{"api": "fix/login"}
	if lost, err := f.svc.Archive(ctx, id, true); err != nil || len(lost) != 0 {
		t.Fatalf("forced Archive = %v, %v", lost, err)
	}
	saved, _ := f.store.Track(ctx, id)
	if !saved.Archived() || !saved.Cleaned() || saved.Status() != track.Closed || !slices.Equal(f.worktrees.discarded, []string{id}) {
		t.Errorf("after Archive: %+v, discarded %v", saved.State, f.worktrees.discarded)
	}
	if saved.Repos[0].Branch != "fix/login" {
		t.Errorf("branch after Archive = %s, want the one the agent renamed it to", saved.Repos[0].Branch)
	}
	if _, err := f.svc.Archive(ctx, id, true); err != nil || len(f.worktrees.discarded) != 1 {
		t.Errorf("archiving twice: %v, discarded %v", err, f.worktrees.discarded)
	}

	if err := f.svc.Unarchive(ctx, id); err != nil {
		t.Fatal(err)
	}
	if saved, _ := f.store.Track(ctx, id); saved.Archived() || !saved.Cleaned() || saved.Status() != track.Done {
		t.Errorf("after Unarchive: %+v, want done", saved.State)
	}
}

func TestArchiveWithoutWorktrees(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	ask, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.End(ctx, ask.Track.ID); err != nil {
		t.Fatal(err)
	}
	f.worktrees.lost = []workspace.Unsaved{{Repo: "api", Changed: 3}}
	if lost, err := f.svc.Archive(ctx, ask.Track.ID, false); err != nil || len(lost) != 0 {
		t.Fatalf("Archive = %v, %v", lost, err)
	}
	if saved, _ := f.store.Track(ctx, ask.Track.ID); !saved.Archived() || len(f.worktrees.discarded) != 0 {
		t.Errorf("after Archive: %+v, discarded %v", saved.State, f.worktrees.discarded)
	}
}

func TestAutoArchive(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	ids := map[string]string{}
	for _, name := range []string{"old", "old-pr", "old-draft", "old-merged", "recent", "open"} {
		created, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: name}, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		ids[name] = created.Track.ID
	}
	ended := func(name string, ago time.Duration) {
		t.Helper()
		if err := f.store.SetState(ctx, ids[name], track.State{ClosedAt: f.svc.now().Add(-ago)}); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"old", "old-pr", "old-draft", "old-merged"} {
		ended(name, ArchiveAfter+time.Hour)
	}
	ended("recent", ArchiveAfter-time.Hour)
	for name, state := range map[string]track.PRState{"old-pr": track.PROpen, "old-draft": track.PRDraft, "old-merged": track.PRMerged} {
		pr, _ := track.ParsePR("https://github.com/acme/api/pull/" + string(rune('1'+len(name))))
		pr.State = state
		if _, err := f.store.AddPR(ctx, ids[name], pr, f.svc.now()); err != nil {
			t.Fatal(err)
		}
	}
	archived := func() []string {
		t.Helper()
		var names []string
		for name, id := range ids {
			if saved, _ := f.store.Track(ctx, id); saved.Archived() {
				names = append(names, name)
			}
		}
		slices.Sort(names)
		return names
	}

	if got, err := f.svc.AutoArchive(ctx); err != nil || len(got) != 0 || len(archived()) != 0 {
		t.Fatalf("AutoArchive while off = %v, %v; archived %v", got, err, archived())
	}

	f.history = settings.History{AutoArchive: true}
	f.worktrees.lost = []workspace.Unsaved{{Repo: "api", Commits: 1}}
	if got, err := f.svc.AutoArchive(ctx); err != nil || len(got) != 0 || len(archived()) != 0 {
		t.Fatalf("AutoArchive skipping work that would be lost = %v, %v; archived %v", got, err, archived())
	}

	f.history.Unsaved = settings.UnsavedKeep
	got, err := f.svc.AutoArchive(ctx)
	slices.Sort(got)
	if err != nil || !slices.Equal(got, []string{"old", "old-merged"}) {
		t.Fatalf("AutoArchive keeping worktrees = %v, %v", got, err)
	}
	if !slices.Equal(archived(), []string{"old", "old-merged"}) || len(f.worktrees.discarded) != 0 {
		t.Errorf("archived %v, discarded %v; want old and old-merged, worktrees kept", archived(), f.worktrees.discarded)
	}

	f.worktrees.lost = nil
	if err := f.svc.Unarchive(ctx, ids["old"]); err != nil {
		t.Fatal(err)
	}
	if got, err := f.svc.AutoArchive(ctx); err != nil || !slices.Equal(got, []string{"old"}) || !slices.Equal(f.worktrees.discarded, []string{ids["old"]}) {
		t.Errorf("AutoArchive with nothing to lose = %v, %v; discarded %v", got, err, f.worktrees.discarded)
	}
}
