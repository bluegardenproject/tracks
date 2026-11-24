package tracks

import (
	"context"
	"testing"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestCreateRecordsTheParent(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	parent, err := f.svc.Create(ctx, Request{Kind: track.Work, Name: "Station bug", Repos: []string{"api"}, Prompt: "Fix it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if parent.Track.Parent != "" {
		t.Errorf("a track made outside a track has parent %q", parent.Track.Parent)
	}

	child, err := f.svc.Create(ctx, Request{Kind: track.Plan, Prompt: "Plan themes", Parent: parent.Track.ID, NoDraft: true}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := f.store.Track(ctx, child.Track.ID)
	if err != nil || saved.Parent != parent.Track.ID || saved.ParentName != "Station bug" {
		t.Errorf("saved parent %q named %q, %v; want %s named Station bug", saved.Parent, saved.ParentName, err, parent.Track.ID)
	}

	gone, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why", Parent: "20261124-000000-gone00", NoDraft: true}, func(string) {})
	if err != nil {
		t.Fatalf("a track started from one that's gone wasn't made: %v", err)
	}
	if saved, err := f.store.Track(ctx, gone.Track.ID); err != nil || saved.Parent != "" {
		t.Errorf("a track started from one that's gone has parent %q, %v; want none", saved.Parent, err)
	}

	if err := f.svc.End(ctx, parent.Track.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Derail(ctx, parent.Track.ID, true); err != nil {
		t.Fatal(err)
	}
	if saved, err := f.store.Track(ctx, child.Track.ID); err != nil || saved.Parent != "" || saved.ParentName != "" {
		t.Errorf("after its parent was derailed the track has parent %q named %q, %v; want none", saved.Parent, saved.ParentName, err)
	}
}
