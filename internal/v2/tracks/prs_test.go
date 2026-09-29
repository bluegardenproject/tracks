package tracks

import (
	"context"
	"errors"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestSeePRs(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	const reviewed, other = "https://github.com/acme/api/pull/9", "https://github.com/acme/api/pull/10"
	got, err := f.svc.Create(ctx, Request{Kind: track.Review, Repos: []string{"api"}, ReviewRef: reviewed + "/files", Prompt: "Review"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id := got.Track.ID
	if err := f.svc.SeePRs(ctx, id, []string{reviewed, "not a link", other, other + "#top"}); err != nil {
		t.Fatal(err)
	}
	tr, err := f.store.Track(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.PRs) != 1 || tr.PRs[0].URL != other || tr.PRs[0].State != track.PROpen {
		t.Errorf("PRs = %+v; want only %s, open", tr.PRs, other)
	}

	var p Problem
	if err := f.svc.SeePRs(ctx, "nope", []string{other}); !errors.As(err, &p) {
		t.Errorf("a PR for a missing track: %v, want a Problem", err)
	}
}
