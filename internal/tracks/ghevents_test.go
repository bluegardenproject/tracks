package tracks

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/track"
)

func TestPREvents(t *testing.T) {
	pr, _ := track.ParsePR("https://github.com/acme/api/pull/7")
	with := func(state track.PRState, h track.Health) track.PR {
		p := pr
		p.State, p.Health = state, h
		return p
	}
	clean := track.Health{Base: "main", Head: "0123456789", Mergeable: track.MergeClean, Checks: track.ChecksPassing}
	conflicts := clean
	conflicts.Mergeable = track.MergeConflicts
	failed := clean
	failed.Checks, failed.Failing = track.ChecksFailing, []string{"lint", "test\n\x1b[31mpush now", "", "a", "b", "c", "d"}
	pushed := failed
	pushed.Head = "fedcba9876"
	both := conflicts
	both.Checks, both.Failing = track.ChecksFailing, []string{"lint"}

	for _, tc := range []struct {
		name   string
		pr     track.PR
		was    track.PRState
		before track.Health
		want   []string
	}{
		{"merged", with(track.PRMerged, clean), track.PROpen, clean, []string{"PR acme/api#7 was merged into main."}},
		{"a draft closed", with(track.PRClosed, clean), track.PRDraft, clean, []string{"PR acme/api#7 was closed without merging."}},
		{"found merged", with(track.PRMerged, clean), "", track.Health{}, nil},
		{"still merged", with(track.PRMerged, conflicts), track.PRMerged, clean, nil},
		{"nothing changed", with(track.PROpen, clean), track.PROpen, clean, nil},
		{"conflicts", with(track.PROpen, conflicts), track.PROpen, clean, []string{"PR acme/api#7 has conflicts with main."}},
		{"still conflicts", with(track.PROpen, conflicts), track.PROpen, conflicts, nil},
		{"found with conflicts", with(track.PRDraft, conflicts), "", track.Health{}, []string{"PR acme/api#7 has conflicts with main."}},
		{"no base", with(track.PROpen, track.Health{Mergeable: track.MergeConflicts}), track.PROpen, clean, []string{"PR acme/api#7 has conflicts with its base."}},
		{"CI failed", with(track.PROpen, failed), track.PROpen, clean,
			[]string{"CI failed on PR acme/api#7 at 0123456: lint, test push now, a, b, c and 1 more."}},
		{"still failing", with(track.PROpen, failed), track.PROpen, failed, nil},
		{"failing again on a new push", with(track.PROpen, pushed), track.PROpen, failed,
			[]string{"CI failed on PR acme/api#7 at fedcba9: lint, test push now, a, b, c and 1 more."}},
		{"both", with(track.PROpen, both), track.PROpen, clean,
			[]string{"PR acme/api#7 has conflicts with main.", "CI failed on PR acme/api#7 at 0123456: lint."}},
	} {
		if got := prEvents(tc.pr, tc.was, tc.before); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestThreadsEvent(t *testing.T) {
	pr, _ := track.ParsePR("https://github.com/acme/api/pull/7")
	for _, tc := range []struct {
		was, n int
		want   string
	}{
		{0, 0, ""}, {3, 2, ""}, {2, 2, ""},
		{0, 1, "1 review thread on PR acme/api#7 waits for an answer."},
		{0, 3, "3 review threads on PR acme/api#7 wait for an answer."},
		{1, 3, "3 review threads on PR acme/api#7 wait for an answer, 2 of them new."},
	} {
		if got := threadsEvent(pr, tc.was, tc.n); got != tc.want {
			t.Errorf("threadsEvent(%d → %d) = %q, want %q", tc.was, tc.n, got, tc.want)
		}
	}
}

func TestGitHubText(t *testing.T) {
	if got := gitHubText("a\tb\n‮c  "); got != "a b c" {
		t.Errorf("gitHubText = %q", got)
	}
	if got := gitHubText(strings.Repeat("é", 100)); len([]rune(got)) != maxGitHubText || !strings.HasSuffix(got, "…") {
		t.Errorf("a long name: %q", got)
	}
}

// githubEvents are the bodies of @github's messages on track id's relay.
func githubEvents(t *testing.T, f *fixture, id string) []string {
	t.Helper()
	log, err := f.store.RelayLog(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range log {
		if m.From == relay.GitHubHandle {
			if m.To != relay.TrackHandle || m.Kind != relay.Event {
				t.Errorf("@github's message %+v isn't an event to the track", m)
			}
			out = append(out, m.Body)
		}
	}
	return out
}

func TestPollPRsPostsGitHubEvents(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gh := &fakeGitHub{}
	f.svc.GitHub = gh
	work, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	const own, asked = "https://github.com/acme/api/pull/1", "https://github.com/acme/api/pull/2"
	branch := work.Track.Repos[0].Branch
	pr, _ := track.ParsePR(own)
	pr.Health = track.Health{Base: "main", Head: "0123456789", Mergeable: track.MergeConflicts}
	gh.branches = map[string][]track.PR{branch: {pr}}
	if err := f.svc.SeePRs(ctx, ask.Track.ID, []string{asked}); err != nil {
		t.Fatal(err)
	}
	gh.states = map[string]track.PRState{asked: track.PROpen}
	gh.health = map[string]track.Health{asked: {Mergeable: track.MergeConflicts}}
	poll := func() {
		t.Helper()
		if err := f.svc.PollPRs(ctx); err != nil {
			t.Fatal(err)
		}
	}

	poll()
	poll()
	if got, want := githubEvents(t, f, work.Track.ID), []string{"PR acme/api#1 has conflicts with main."}; !slices.Equal(got, want) {
		t.Errorf("the Work track's events = %q, want %q, once", got, want)
	}
	if got := githubEvents(t, f, ask.Track.ID); len(got) != 0 {
		t.Errorf("an Ask track got events: %q", got)
	}

	pr.Health = track.Health{Base: "main", Head: "0123456789", Mergeable: track.MergeClean,
		Checks: track.ChecksFailing, Failing: []string{"lint"}}
	gh.branches[branch] = []track.PR{pr}
	poll()
	pr.State = track.PRMerged
	gh.branches[branch] = []track.PR{pr}
	poll()
	want := []string{"PR acme/api#1 has conflicts with main.", "CI failed on PR acme/api#1 at 0123456: lint.",
		"PR acme/api#1 was merged into main."}
	if got := githubEvents(t, f, work.Track.ID); !slices.Equal(got, want) {
		t.Errorf("the Work track's events = %q, want %q", got, want)
	}
	unread, err := f.store.RelayUnread(ctx, work.Track.ID, relay.TrackHandle)
	if err != nil || len(unread) != 3 {
		t.Errorf("the track's unread = %+v, %v; want @github's 3 events, which wake its agent", unread, err)
	}

	if err := f.svc.End(ctx, work.Track.ID); err != nil {
		t.Fatal(err)
	}
	pr.State, pr.Number, pr.URL = track.PROpen, 3, "https://github.com/acme/api/pull/3"
	gh.branches[branch] = []track.PR{pr}
	poll()
	if got := githubEvents(t, f, work.Track.ID); len(got) != len(want) {
		t.Errorf("an ended track got events: %q", got[len(want):])
	}
}

func TestCountCommentsPostsGitHubEvents(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gh := &fakeGitHub{}
	f.svc.GitHub = gh
	work, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	const url = "https://github.com/acme/api/pull/1"
	if err := f.svc.SeePRs(ctx, work.Track.ID, []string{url}); err != nil {
		t.Fatal(err)
	}
	count := func(n int) {
		t.Helper()
		gh.threads = map[string]int{url: n}
		if err := f.svc.CountComments(ctx); err != nil {
			t.Fatal(err)
		}
	}
	count(2)
	count(1)
	count(3)
	count(3)
	want := []string{"3 review threads on PR acme/api#1 wait for an answer, 2 of them new."}
	if got := githubEvents(t, f, work.Track.ID); !slices.Equal(got, want) {
		t.Errorf("events = %q, want %q: none for the first count, nor for fewer", got, want)
	}
}
