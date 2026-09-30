package tracks

import (
	"context"
	"slices"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// heard collects the notices f's service sends, as "event title body".
func heard(f *fixture) *[]string {
	var got []string
	f.svc.Notify = func(n Notice) { got = append(got, n.Event+" "+n.Title+" "+n.Body) }
	return &got
}

func TestStatusNotices(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	got := heard(f)
	c, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id, name := c.Track.ID, c.Track.Name
	for _, e := range []track.Event{track.AgentWaiting, track.AgentWaiting, track.AgentWorking, track.AgentFailed, track.AgentWorking} {
		if err := f.svc.Report(ctx, id, e); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{
		"action_required Tracks: " + name + " needs you Its agent is waiting for an answer.",
		"error Tracks: " + name + " failed Its agent exited with an error.",
	}
	if !slices.Equal(*got, want) {
		t.Errorf("notices %q, want %q: one on becoming action required and one on becoming error, none for the rest", *got, want)
	}

	*got = nil
	c, err = f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Report(ctx, c.Track.ID, track.AgentExited); err != nil {
		t.Fatal(err)
	}
	if len(*got) != 0 {
		t.Errorf("creating a track and its agent exiting cleanly sent %q; want nothing", *got)
	}
}

func TestPRNotices(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gh := &fakeGitHub{}
	f.svc.GitHub = gh
	got := heard(f)
	work, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id, name := work.Track.ID, work.Track.Name
	const byHook, byHand, old = "https://github.com/acme/api/pull/1", "https://github.com/acme/api/pull/2", "https://github.com/acme/api/pull/3"
	for range 2 {
		if err := f.svc.SeePRs(ctx, id, []string{byHook}); err != nil {
			t.Fatal(err)
		}
	}
	draft, _ := track.ParsePR(byHand)
	draft.State = track.PRDraft
	merged, _ := track.ParsePR(old)
	merged.State = track.PRMerged
	gh.branches = map[string][]track.PR{work.Track.Repos[0].Branch: {draft, merged}}
	gh.states = map[string]track.PRState{byHook: track.PRMerged}
	for range 2 {
		if err := f.svc.PollPRs(ctx); err != nil {
			t.Fatal(err)
		}
	}
	gh.branches = map[string][]track.PR{work.Track.Repos[0].Branch: {{URL: byHand, Repo: "acme/api", Number: 2, State: track.PRClosed}, merged}}
	if err := f.svc.PollPRs(ctx); err != nil {
		t.Fatal(err)
	}

	want := []string{
		"pr_opened Tracks: " + name + " opened a PR acme/api#1",
		"pr_opened Tracks: " + name + " opened a PR acme/api#2",
		"pr_settled Tracks: " + name + "'s PR was merged acme/api#1",
		"pr_settled Tracks: " + name + "'s PR was closed acme/api#2",
	}
	if !slices.Equal(*got, want) {
		t.Errorf("notices %q\nwant %q: a PR opening once, a known one settling once, none for one found merged", *got, want)
	}
}
