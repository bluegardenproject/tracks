package tracks

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// stationDrafts are Station's draft rows.
func stationDrafts(t *testing.T, f *fixture) []Listed {
	t.Helper()
	listed, _, err := f.svc.Station(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(listed, func(l Listed) bool { return l.Draft == nil })
}

func TestDrafts(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	req := Request{Kind: track.Work, Repos: []string{"nope"}, Prompt: "Fix the login\nand the logout", Engine: "claude", Model: "opus"}
	if _, err := f.svc.Create(ctx, req, func(string) {}); err == nil {
		t.Fatal("creating from a repo that doesn't exist should fail")
	}
	drafts := stationDrafts(t, f)
	if len(drafts) != 1 {
		t.Fatalf("Station lists %d drafts, want the failed creation", len(drafts))
	}
	d := drafts[0]
	if d.Status() != track.Draft || d.Title != "Fix the login" || d.Kind != track.Work || len(d.Repos) != 1 || d.Repos[0].Name != "nope" || d.Draft.Error == "" {
		t.Errorf("the draft row = %+v, %+v; want its status, the prompt's first line, the kind, the repo and why", d.Track, d.Draft)
	}
	first := d.Draft.Error

	again, err := f.svc.Draft(ctx, d.ID)
	if err != nil || again.Draft != d.ID || again.Prompt != req.Prompt || again.Model != "opus" || !slices.Equal(again.Repos, req.Repos) {
		t.Fatalf("Draft = %+v, %v; want the request, with the draft's ID", again, err)
	}
	again.Repos = []string{"gone"}
	if _, err := f.svc.Create(ctx, again, func(string) {}); err == nil {
		t.Fatal("the retry should fail too")
	}
	if drafts := stationDrafts(t, f); len(drafts) != 1 || drafts[0].Draft.Error == first || !strings.Contains(drafts[0].Draft.Error, "gone") {
		t.Errorf("after a failed retry, drafts %+v; want the one, with the new reason", drafts)
	}

	again.Repos = []string{"api"}
	if _, err := f.svc.Create(ctx, again, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if drafts := stationDrafts(t, f); len(drafts) != 0 {
		t.Errorf("after the creation succeeded, drafts %+v; want none", drafts)
	}
}

func TestDraftsUnderAFilter(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if _, err := f.svc.Create(ctx, Request{Kind: track.Ask, Name: "Why", Prompt: "Why", Engine: "nope"}, func(string) {}); err == nil {
		t.Fatal("creating on an unknown engine should fail")
	}
	for _, c := range []struct {
		name   string
		filter track.Filter
		shown  bool
	}{
		{"none", track.Filter{}, true},
		{"draft", track.Filter{Statuses: []string{"draft", "active"}}, true},
		{"draft without a PR", track.Filter{Statuses: []string{"draft"}, PRStatuses: []string{"none"}}, true},
		{"another status", track.Filter{Statuses: []string{"active"}}, false},
		{"a PR status", track.Filter{Statuses: []string{"draft"}, PRStatuses: []string{"open"}}, false},
		{"archived", track.Filter{Archived: true, Statuses: []string{"draft"}}, false},
		{"started today", track.Filter{Started: track.Today}, false},
	} {
		if err := f.svc.SetFilter(ctx, c.filter); err != nil {
			t.Fatal(err)
		}
		if got := len(stationDrafts(t, f)) == 1; got != c.shown {
			t.Errorf("%s: the draft shown = %v, want %v", c.name, got, c.shown)
		}
	}
}

func TestDiscardDraft(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if _, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why", Engine: "nope"}, func(string) {}); err == nil {
		t.Fatal("creating on an unknown engine should fail")
	}
	id := stationDrafts(t, f)[0].ID
	if err := f.svc.DiscardDraft(ctx, id); err != nil {
		t.Fatal(err)
	}
	if len(stationDrafts(t, f)) != 0 {
		t.Error("the discarded draft is still listed")
	}
	var p Problem
	if err := f.svc.DiscardDraft(ctx, id); !errors.As(err, &p) {
		t.Errorf("discarding it again = %v, want a problem", err)
	}
	if _, err := f.svc.Draft(ctx, id); !errors.As(err, &p) {
		t.Errorf("Draft of a discarded draft = %v, want a problem", err)
	}
}

func TestFailure(t *testing.T) {
	if got := Failure(Problem("Add Cursor on the Engines tab.")); got != "Add Cursor on the Engines tab." {
		t.Errorf("a problem is told as %q", got)
	}
	if got := Failure(errors.New("git: exit 128")); got != "Couldn't create the track: git: exit 128" {
		t.Errorf("an error is told as %q", got)
	}
}
