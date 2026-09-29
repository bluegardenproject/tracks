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

	f.worktrees.unsaved = []workspace.Unsaved{{Repo: "api", Changed: 3}}
	if unsaved, err := f.svc.Archive(ctx, id, false); err != nil || len(unsaved) != 1 {
		t.Fatalf("Archive = %v, %v; want the unsaved work", unsaved, err)
	}
	if saved, _ := f.store.Track(ctx, id); saved.Archived() || saved.Cleaned() {
		t.Fatalf("Archive changed %+v with unsaved work", saved.State)
	}
	if unsaved, err := f.svc.Archive(ctx, id, true); err != nil || len(unsaved) != 0 {
		t.Fatalf("forced Archive = %v, %v", unsaved, err)
	}
	saved, _ := f.store.Track(ctx, id)
	if !saved.Archived() || !saved.Cleaned() || !slices.Equal(f.worktrees.cleaned, []string{id}) {
		t.Errorf("after Archive: %+v, removed %v", saved.State, f.worktrees.cleaned)
	}
	if _, err := f.svc.Archive(ctx, id, true); err != nil || len(f.worktrees.cleaned) != 1 {
		t.Errorf("archiving twice: %v, removed %v", err, f.worktrees.cleaned)
	}

	if err := f.svc.Unarchive(ctx, id); err != nil {
		t.Fatal(err)
	}
	if saved, _ := f.store.Track(ctx, id); saved.Archived() || !saved.Cleaned() {
		t.Errorf("after Unarchive: %+v", saved.State)
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
	f.worktrees.unsaved = []workspace.Unsaved{{Repo: "api", Changed: 3}}
	if unsaved, err := f.svc.Archive(ctx, ask.Track.ID, false); err != nil || len(unsaved) != 0 {
		t.Fatalf("Archive = %v, %v", unsaved, err)
	}
	if saved, _ := f.store.Track(ctx, ask.Track.ID); !saved.Archived() || len(f.worktrees.cleaned) != 0 {
		t.Errorf("after Archive: %+v, removed %v", saved.State, f.worktrees.cleaned)
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
	f.worktrees.unsaved = []workspace.Unsaved{{Repo: "api", Changed: 1}}
	if got, err := f.svc.AutoArchive(ctx); err != nil || len(got) != 0 || len(archived()) != 0 {
		t.Fatalf("AutoArchive skipping unsaved work = %v, %v; archived %v", got, err, archived())
	}

	f.history.Unsaved = settings.UnsavedKeep
	got, err := f.svc.AutoArchive(ctx)
	slices.Sort(got)
	if err != nil || !slices.Equal(got, []string{"old", "old-merged"}) {
		t.Fatalf("AutoArchive keeping worktrees = %v, %v", got, err)
	}
	if !slices.Equal(archived(), []string{"old", "old-merged"}) || len(f.worktrees.cleaned) != 0 {
		t.Errorf("archived %v, removed %v; want old and old-merged, worktrees kept", archived(), f.worktrees.cleaned)
	}
}
