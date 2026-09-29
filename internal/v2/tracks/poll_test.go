package tracks

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

type fakeGitHub struct {
	states   map[string]track.PRState
	branches map[string][]track.PR // by branch
	fail     error
	asked    []string
}

func (g *fakeGitHub) PR(_ context.Context, url string) (track.PRState, error) {
	g.asked = append(g.asked, "view "+url)
	if g.fail != nil {
		return "", g.fail
	}
	return g.states[url], nil
}

func (g *fakeGitHub) BranchPRs(_ context.Context, _, branch string) ([]track.PR, error) {
	g.asked = append(g.asked, "list "+branch)
	if g.fail != nil {
		return nil, g.fail
	}
	return g.branches[branch], nil
}

func TestPollPRs(t *testing.T) {
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
	branch := work.Track.Repos[0].Branch
	const byHook, byHand, settled = "https://github.com/acme/api/pull/1", "https://github.com/acme/api/pull/2", "https://github.com/acme/web/pull/3"
	if err := f.svc.SeePRs(ctx, ask.Track.ID, []string{byHook, settled}); err != nil {
		t.Fatal(err)
	}
	merged, _ := track.ParsePR(settled)
	merged.State = track.PRMerged
	if err := f.store.SavePR(ctx, ask.Track.ID, merged, f.svc.now()); err != nil {
		t.Fatal(err)
	}
	draft, _ := track.ParsePR(byHand)
	draft.State = track.PRDraft
	gh.branches = map[string][]track.PR{branch: {draft}}
	gh.states = map[string]track.PRState{byHook: track.PRClosed}

	if err := f.svc.PollPRs(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"list " + branch, "view " + byHook}; !slices.Equal(gh.asked, want) {
		t.Errorf("asked %q, want %q: the branch, then the unsettled PRs not found on it", gh.asked, want)
	}
	prs := func(id string) []track.PR {
		t.Helper()
		tr, err := f.store.Track(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return tr.PRs
	}
	if got := prs(work.Track.ID); len(got) != 1 || got[0].URL != byHand || got[0].State != track.PRDraft || got[0].CheckedAt.IsZero() {
		t.Errorf("the Work track's PRs = %+v; want the one found on its branch, a draft", got)
	}
	if got := prs(ask.Track.ID); len(got) != 2 || got[0].State != track.PRClosed || got[1].State != track.PRMerged {
		t.Errorf("the Ask track's PRs = %+v; want the hook's closed, the settled one left merged", got)
	}

	gh.asked = nil
	if err := f.svc.End(ctx, work.Track.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.PollPRs(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"list " + branch}; !slices.Equal(gh.asked, want) {
		t.Errorf("after ending the Work track, asked %q, want %q: an ended track's branch still gets PRs", gh.asked, want)
	}

	gh.fail, gh.asked = ErrNoGH, nil
	if err := f.svc.PollPRs(ctx); !errors.Is(err, ErrNoGH) || len(gh.asked) != 1 {
		t.Errorf("without gh: %v after %q; want ErrNoGH at the first call", err, gh.asked)
	}
	gh.fail = errors.New("HTTP 401")
	if err := f.svc.PollPRs(ctx); err == nil {
		t.Error("a failing gh is reported")
	}
}
