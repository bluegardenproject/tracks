package tracks

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/track"
)

type fakeGitHub struct {
	states   map[string]track.PRState
	health   map[string]track.Health // by PR URL
	missing  map[string]bool         // PRs Poll can't read, by URL
	repos    int                     // RepoOf's calls
	branches map[string][]track.PR   // by branch
	fail     error
	asked    []string
	threads  map[string]int // by PR URL
	// ReviewPR's answer.
	head        string
	reviewNotes []string
	reviewErr   error
	// PRTitle's answer, or titleErr.
	title    string
	titleErr error
}

func (g *fakeGitHub) PRTitle(_ context.Context, pr track.PR) (string, error) {
	g.asked = append(g.asked, "title "+pr.URL)
	return g.title, g.titleErr
}

func (g *fakeGitHub) ReviewPR(_ context.Context, pr track.PR) (review.PR, string, []string, error) {
	if g.reviewErr != nil {
		return review.PR{}, "", nil, g.reviewErr
	}
	return review.PR{URL: pr.URL, Title: "Fix the cache"}, g.head, g.reviewNotes, nil
}

func (g *fakeGitHub) Poll(_ context.Context, q Query) (Answer, error) {
	a := Answer{Branches: map[BranchQuery][]PRFacts{}, PRs: map[string]PRFacts{}}
	if g.fail != nil {
		g.asked = append(g.asked, "poll")
		return a, g.fail
	}
	for _, b := range q.Branches {
		g.asked = append(g.asked, "list "+b.Branch)
		list := []PRFacts{}
		for _, pr := range g.branches[b.Branch] {
			list = append(list, PRFacts{PR: pr, Threads: g.threads[pr.URL]})
		}
		a.Branches[b] = list
	}
	for _, pr := range q.PRs {
		g.asked = append(g.asked, "view "+pr.URL)
		if g.missing[pr.URL] {
			continue
		}
		p := pr
		if state, ok := g.states[pr.URL]; ok {
			p.State = state
		}
		p.Health = g.health[pr.URL]
		a.PRs[pr.URL] = PRFacts{PR: p, Threads: g.threads[pr.URL]}
	}
	if len(g.missing) > 0 {
		return a, errors.New("GitHub didn't answer for some")
	}
	return a, nil
}

func (g *fakeGitHub) RepoOf(_ context.Context, dir string) (string, error) {
	if g.fail != nil {
		g.asked = append(g.asked, "repo")
		return "", g.fail
	}
	g.repos++
	return "acme/" + filepath.Base(dir), nil
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
	if _, err := f.store.SavePR(ctx, ask.Track.ID, merged, f.svc.now()); err != nil {
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
	if want := []string{"list " + branch, "view " + byHand}; !slices.Equal(gh.asked, want) {
		t.Errorf("after ending the Work track, asked %q, want %q: an ended track's branch still gets PRs, and its draft is unsettled", gh.asked, want)
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

func TestPollKeepsWhatItCouldntRead(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gh := &fakeGitHub{}
	f.svc.GitHub = gh
	work, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	const url = "https://github.com/acme/web/pull/5"
	if err := f.svc.SeePRs(ctx, work.Track.ID, []string{url}); err != nil {
		t.Fatal(err)
	}
	poll := func() error {
		if err := f.svc.PollPRs(ctx); err != nil {
			return err
		}
		return f.svc.CountComments(ctx)
	}
	threads := func() int {
		f.svc.mu.Lock()
		defer f.svc.mu.Unlock()
		return f.svc.comments[work.Track.ID][url].Open
	}
	gh.threads = map[string]int{url: 2}
	if err := poll(); err != nil {
		t.Fatal(err)
	}
	gh.threads, gh.missing = map[string]int{url: 0}, map[string]bool{url: true}
	if err := poll(); err == nil {
		t.Error("a PR GitHub didn't answer for is no error")
	}
	if n := threads(); n != 2 {
		t.Errorf("a PR the poll couldn't read has %d threads, want its last 2", n)
	}
	if gh.repos != 1 {
		t.Errorf("gh asked for the repo %d times over two polls, want once", gh.repos)
	}

	gh.missing = nil
	gh.states = map[string]track.PRState{url: track.PRMerged}
	if err := poll(); err != nil {
		t.Fatal(err)
	}
	f.svc.mu.Lock()
	_, kept := f.svc.threads[url]
	f.svc.mu.Unlock()
	if kept {
		t.Error("a merged PR's threads are kept")
	}
}
