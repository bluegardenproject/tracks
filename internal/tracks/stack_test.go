package tracks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/track"
)

// stackedFixture is sideFixture with a side track stacked on its first
// side track, whose branch is pushed.
func stackedFixture(t *testing.T) (f *fixture, main, lower, upper track.Track) {
	t.Helper()
	ctx := context.Background()
	f, main, lower = sideFixture(t)
	f.worktrees.pushed = map[string]bool{lower.Repos[0].Branch: true}
	created, err := f.svc.Create(ctx, Request{Kind: track.Work, Name: "relay-cli", Prompt: "A CLI on the store",
		Parent: main.ID, Side: true, NoDraft: true, On: lower.Side}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	upper, err = f.store.Track(ctx, created.Track.ID)
	if err != nil {
		t.Fatal(err)
	}
	return f, main, lower, upper
}

func TestStackedSideTrack(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	if got := upper.Repos[0].Base; got != lower.Repos[0].Branch {
		t.Errorf("the stacked side track's base %q, want %s's branch %q", got, lower.Name, lower.Repos[0].Branch)
	}
	spec := f.svc.sideSpec(ctx, upper)
	if spec == nil || len(spec.Stacked) != 1 || spec.Stacked[0].On != "relay-store" || spec.Stacked[0].Branch != lower.Repos[0].Branch || spec.Stacked[0].Repo != "api" {
		t.Errorf("its prompt's stack: %+v", spec)
	}
	if spec := f.svc.sideSpec(ctx, lower); spec == nil || len(spec.Stacked) != 0 {
		t.Errorf("an unstacked side track's prompt names a stack: %+v", spec)
	}

	onMain, err := f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "Fan in", Parent: main.ID, Side: true, NoDraft: true, On: "track"}, func(string) {})
	if !errors.As(err, new(Problem)) || !strings.Contains(err.Error(), "Push api-cache's branch") {
		t.Errorf("stacked on the main track's unpushed branch: %+v, %v", onMain, err)
	}
	f.worktrees.pushed[main.Repos[0].Branch] = true
	onMain, err = f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "Fan in", Parent: main.ID, Side: true, NoDraft: true, On: "track"}, func(string) {})
	if err != nil || onMain.Track.Repos[0].Base != main.Repos[0].Branch {
		t.Errorf("stacked on the main track: %+v, %v; want its branch as base", onMain.Track.Repos, err)
	}

	for _, tc := range []struct {
		req Request
		why string
	}{
		{Request{Kind: track.Work, Prompt: "x", Parent: main.ID, Side: true, NoDraft: true, On: "side-999999"}, "tracks side list"},
		{Request{Kind: track.Work, Prompt: "x", NoDraft: true, On: "track"}, "stacks side tracks only"},
	} {
		if _, err := f.svc.Create(ctx, tc.req, func(string) {}); !errors.As(err, new(Problem)) || !strings.Contains(err.Error(), tc.why) {
			t.Errorf("--on %s: %v, want %q", tc.req.On, err, tc.why)
		}
	}

	pr, _ := track.ParsePR("https://github.com/acme/api/pull/232")
	pr.State = track.PRMerged
	if _, err := f.store.SavePR(ctx, lower.ID, pr, f.svc.now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "x", Parent: main.ID, Side: true, NoDraft: true, On: lower.Side}, func(string) {}); !errors.As(err, new(Problem)) || !strings.Contains(err.Error(), "merged or closed") {
		t.Errorf("stacked on a side track whose PR merged: %v", err)
	}
}

func TestRestack(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	f.svc.GitHub = &fakeGitHub{}
	pr := mergedBelow(lower)
	if _, err := f.store.SavePR(ctx, lower.ID, pr, f.svc.now()); err != nil {
		t.Fatal(err)
	}
	found := &pollFound{}
	if err := f.svc.prHealth(ctx, lower.ID, pr, track.PROpen, nil, found); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 0 {
		t.Errorf("restacked before the poll saved every PR: %q", got)
	}
	if err := f.svc.restackMerged(ctx, found.merged); err != nil {
		t.Fatal(err)
	}
	now, err := f.store.Track(ctx, upper.ID)
	if err != nil || now.Repos[0].Base != "main" {
		t.Errorf("after the PR below merged, the base is %q, %v; want main", now.Repos[0].Base, err)
	}
	want := "restack: " + actions.SideRestack("acme/api#232", "relay-store", "main", "0123456")
	if got := waitingFor(t, f, upper.ID); len(got) != 1 || got[0] != want {
		t.Errorf("waiting for the stacked side track: %q, want %q", got, want)
	}
	if got := fromGitHub(t, f, main.ID); len(got) != 1 || !strings.HasPrefix(got[0], "notice: @"+upper.Side) || !strings.Contains(got[0], "Tracks tells it to restack") {
		t.Errorf("the main track's relay: %q", got)
	}
	if got := waitingFor(t, f, lower.ID); len(got) != 0 {
		t.Errorf("the merged side track was told something: %q", got)
	}
	if spec := f.svc.sideSpec(ctx, now); len(spec.Stacked) != 0 {
		t.Errorf("restacked onto main, its prompt still names a stack: %+v", spec.Stacked)
	}
}

func TestAnEndedStackedSideTrackIsTheMainTracks(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	if err := f.svc.Report(ctx, upper.ID, track.Ended); err != nil {
		t.Fatal(err)
	}
	f.svc.GitHub = &fakeGitHub{}
	if err := f.svc.restack(ctx, lower.ID, mergedBelow(lower)); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 0 {
		t.Errorf("instructed an ended side track: %q", got)
	}
	if got := fromGitHub(t, f, main.ID); len(got) == 0 || !strings.HasPrefix(got[len(got)-1], "event: @"+upper.Side) {
		t.Errorf("the main track's relay: %q; want an event, for it to decide", got)
	}
}

// mergedBelow is lower's PR in acme/api, merged into main from its
// branch.
func mergedBelow(lower track.Track) track.PR {
	pr, _ := track.ParsePR("https://github.com/acme/api/pull/232")
	pr.State = track.PRMerged
	pr.Health = track.Health{Base: "main", Head: "0123456789", HeadBranch: lower.Repos[0].Branch}
	return pr
}

// A restack goes only to the repo the PR is in, and not to a side track
// whose PRs there are merged already, as a fan-in's; one that merged
// while its restack waited drops it.
func TestRestackOnlyWhatIsStacked(t *testing.T) {
	ctx := context.Background()
	f, _, lower, upper := stackedFixture(t)
	f.svc.GitHub = &fakeGitHub{}
	other := mergedBelow(lower)
	other.Repo, other.URL = "acme/web", "https://github.com/acme/web/pull/9"
	if err := f.svc.restack(ctx, lower.ID, other); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 0 {
		t.Errorf("a PR in another repo restacked it: %q", got)
	}
	own, _ := track.ParsePR("https://github.com/acme/api/pull/240")
	own.State = track.PRMerged
	if _, err := f.store.SavePR(ctx, upper.ID, own, f.svc.now()); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.restack(ctx, lower.ID, mergedBelow(lower)); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 0 {
		t.Errorf("a side track whose PR merged already was restacked: %q", got)
	}
}

func TestAMootRestackIsDropped(t *testing.T) {
	ctx := context.Background()
	f, _, lower, upper := stackedFixture(t)
	f.svc.GitHub = &fakeGitHub{}
	below := mergedBelow(lower)
	if err := f.svc.restack(ctx, lower.ID, below); err != nil {
		t.Fatal(err)
	}
	own, _ := track.ParsePR("https://github.com/acme/api/pull/240")
	own.State = track.PRMerged
	if _, err := f.store.SavePR(ctx, upper.ID, own, f.svc.now()); err != nil {
		t.Fatal(err)
	}
	now, _ := f.store.Track(ctx, upper.ID)
	waiting, _ := f.store.WaitingInstructions(ctx)
	typed, held, err := f.svc.typeInstruction(ctx, now, "@x", waiting[0])
	if err != nil || typed || held {
		t.Errorf("a restack whose side track's PR merged: typed %v, held %v, %v", typed, held, err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 0 {
		t.Errorf("still waiting: %q", got)
	}
}

// A base branch that isn't safe in a command leaves the restack to the
// main track.
func TestAnUnsafeBaseIsTheMainTracks(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	f.svc.GitHub = &fakeGitHub{}
	pr := mergedBelow(lower)
	pr.Health.Base = "main$(curl evil)"
	if err := f.svc.restack(ctx, lower.ID, pr); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 0 {
		t.Errorf("instructed with an unsafe base: %q", got)
	}
	if got := fromGitHub(t, f, main.ID); len(got) != 1 || !strings.HasPrefix(got[0], "event: ") || !strings.Contains(got[0], "you decide") {
		t.Errorf("the main track's relay: %q", got)
	}
	if now, _ := f.store.Track(ctx, upper.ID); now.Repos[0].Base != lower.Repos[0].Branch {
		t.Errorf("the base became %q", now.Repos[0].Base)
	}
}

// Two side tracks stacked on the same PR: one ending keeps the other's
// restack.
func TestAnEndingSiblingKeepsTheOthersRestack(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	second, err := f.svc.Create(ctx, Request{Kind: track.Work, Name: "relay-docs", Prompt: "Docs", Parent: main.ID, Side: true, NoDraft: true, On: lower.Side}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	f.svc.GitHub = &fakeGitHub{}
	if err := f.svc.restack(ctx, lower.ID, mergedBelow(lower)); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Report(ctx, upper.ID, track.Ended); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, second.Track.ID); len(got) != 1 {
		t.Errorf("the other side track's restack: %q; want it kept", got)
	}
}

// A lower track whose agent renamed its branch is stacked on the new
// one, which is kept.
func TestStackingOnARenamedBranch(t *testing.T) {
	ctx := context.Background()
	f, main, lower := sideFixture(t)
	f.worktrees.renamed = map[string]string{"api": "side/relay-store"}
	f.worktrees.pushed = map[string]bool{"side/relay-store": true}
	created, err := f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "A CLI", Parent: main.ID, Side: true, NoDraft: true, On: lower.Side}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if got := created.Track.Repos[0].Base; got != "side/relay-store" {
		t.Errorf("stacked on %q, want the renamed branch", got)
	}
	if now, err := f.store.Track(ctx, lower.ID); err != nil || now.Repos[0].Branch != "side/relay-store" {
		t.Errorf("the lower track's branch is %q, %v; want the renamed one kept", now.Repos[0].Branch, err)
	}
}

func TestFinalBase(t *testing.T) {
	pr := func(n int, repo, head, base string) mergedPR {
		p, _ := track.ParsePR(fmt.Sprintf("https://github.com/%s/pull/%d", repo, n))
		p.Health = track.Health{HeadBranch: head, Base: base}
		return mergedPR{pr: p}
	}
	a := pr(1, "acme/api", "a", "main")
	b := pr(2, "acme/api", "b", "a")
	c := pr(3, "acme/api", "c", "b")
	other := pr(4, "acme/web", "b", "release")
	for _, tc := range []struct {
		name   string
		of     mergedPR
		merged []mergedPR
		want   string
	}{
		{"alone", b, []mergedPR{b}, "a"},
		{"the one below merged too", b, []mergedPR{a, b}, "main"},
		{"two below merged too", c, []mergedPR{c, a, b}, "main"},
		{"another repo's branch of that name", c, []mergedPR{c, other}, "b"},
	} {
		if got := finalBase(tc.of.pr, tc.merged); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A restack drops what it makes moot for its side track: an earlier
// restack, its conflicts' rebase and the main track's question on it.
func TestARestackDropsWhatItMakesMoot(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	f.svc.GitHub = &fakeGitHub{}
	const own = "https://github.com/acme/api/pull/240"
	if err := f.svc.SeePRs(ctx, upper.ID, []string{own}); err != nil {
		t.Fatal(err)
	}
	for _, in := range []struct{ id, kind, pr string }{
		{upper.ID, settings.EventConflicts, own},
		{upper.ID, instructionRestack, "https://github.com/acme/api/pull/100"},
		{main.ID, askKind(settings.EventConflicts), own},
		{upper.ID, settings.EventCIFailed, own},
	} {
		if err := f.svc.instruct(ctx, in.id, in.kind, in.pr, "old"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.svc.restack(ctx, lower.ID, mergedBelow(lower)); err != nil {
		t.Fatal(err)
	}
	got := waitingFor(t, f, upper.ID)
	if len(got) != 2 || !strings.HasPrefix(got[0], "ci_failed: ") || !strings.HasPrefix(got[1], "restack: The PR below") {
		t.Errorf("waiting for the side track: %q; want CI's and the new restack", got)
	}
	if got := waitingFor(t, f, main.ID); len(got) != 0 {
		t.Errorf("waiting for the main track: %q", got)
	}
}

// A restack in one repo keeps a waiting one in another.
func TestRestacksInTwoReposAreKept(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	f.svc.GitHub = &fakeGitHub{}
	if err := f.svc.instruct(ctx, upper.ID, instructionRestack, "https://github.com/acme/web/pull/7", "web's restack"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.dropBeforeRestack(ctx, upper, main.ID, "acme/api"); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 1 {
		t.Errorf("api's restack dropped web's: %q", got)
	}
	_ = lower
}

// A side track that ends with a restack its own merged PR made moot
// drops it without troubling its main track.
func TestAnEndingSideTracksMootRestackGoesQuietly(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	f.svc.GitHub = &fakeGitHub{}
	if err := f.svc.restack(ctx, lower.ID, mergedBelow(lower)); err != nil {
		t.Fatal(err)
	}
	own, _ := track.ParsePR("https://github.com/acme/api/pull/240")
	own.State = track.PRMerged
	if _, err := f.store.SavePR(ctx, upper.ID, own, f.svc.now()); err != nil {
		t.Fatal(err)
	}
	before := len(fromGitHub(t, f, main.ID))
	if err := f.svc.Report(ctx, upper.ID, track.Ended); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 0 {
		t.Errorf("still waiting: %q", got)
	}
	if got := fromGitHub(t, f, main.ID); len(got) != before {
		t.Errorf("the main track heard of a moot restack: %q", got[before:])
	}
}

// --on track builds only on a Work main track's branch.
func TestStackingOnlyOnAWorkTrack(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []track.Kind{track.Plan, track.Review} {
		f := newFixture(t)
		req := Request{Kind: kind, Name: "main-" + string(kind), Repos: []string{"api"}, Prompt: "Look"}
		if kind == track.Review {
			req.ReviewRef = "feat/x"
		}
		main, err := f.svc.Create(ctx, req, func(string) {})
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		f.worktrees.pushed = map[string]bool{"feat/x": true}
		_, err = f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "Part", Parent: main.Track.ID, Side: true, NoDraft: true, On: "track"}, func(string) {})
		if !errors.As(err, new(Problem)) || !strings.Contains(err.Error(), "no branch of its own") {
			t.Errorf("--on track from a %s track: %v", kind, err)
		}
	}
}

// A conflicts event a waiting restack solves is no event of its own.
func TestConflictsWhileARestackWaits(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	f.svc.GitHub = &fakeGitHub{}
	if err := f.svc.restack(ctx, lower.ID, mergedBelow(lower)); err != nil {
		t.Fatal(err)
	}
	before := len(fromGitHub(t, f, main.ID))
	own, _ := track.ParsePR("https://github.com/acme/api/pull/240")
	if err := f.svc.githubEvent(ctx, upper, ghEvent{Kind: settings.EventConflicts, PR: own, Body: "conflicts"}); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 1 || !strings.HasPrefix(got[0], "restack: ") {
		t.Errorf("waiting: %q; want the restack alone", got)
	}
	if got := fromGitHub(t, f, main.ID); len(got) != before {
		t.Errorf("the main track heard of it: %q", got[before:])
	}
}

// The poll that sees the PR below merge, and the PR above conflict as
// GitHub retargets it, restacks it and tells no conflict.
func TestAMergeAndItsConflictInOnePoll(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	gh := &fakeGitHub{}
	f.svc.GitHub = gh
	below, _ := track.ParsePR("https://github.com/acme/api/pull/232")
	below.Health = track.Health{Base: "main", Head: "0123456789", HeadBranch: lower.Repos[0].Branch, Mergeable: track.MergeClean}
	above, _ := track.ParsePR("https://github.com/acme/api/pull/240")
	above.Health = track.Health{Base: lower.Repos[0].Branch, Head: "abcdef0123", HeadBranch: upper.Repos[0].Branch, Mergeable: track.MergeClean}
	gh.branches = map[string][]track.PR{lower.Repos[0].Branch: {below}, upper.Repos[0].Branch: {above}}
	if err := f.svc.PollPRs(ctx); err != nil {
		t.Fatal(err)
	}
	below.State = track.PRMerged
	above.Health.Base, above.Health.Mergeable = "main", track.MergeConflicts
	gh.branches = map[string][]track.PR{lower.Repos[0].Branch: {below}, upper.Repos[0].Branch: {above}}
	if err := f.svc.PollPRs(ctx); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 1 || !strings.HasPrefix(got[0], "restack: ") {
		t.Errorf("waiting for the side track above: %q; want the restack alone", got)
	}
	for _, m := range fromGitHub(t, f, main.ID) {
		if strings.Contains(m, "has conflicts") {
			t.Errorf("the main track heard of the conflict the restack solves: %q", m)
		}
	}
}

// An unsafe name with the main track ended goes to the side track.
func TestAnUnsafeBaseWithTheMainTrackEnded(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	f.svc.GitHub = &fakeGitHub{}
	if err := f.store.SetState(ctx, main.ID, main.State.Apply(track.Ended, f.svc.now())); err != nil {
		t.Fatal(err)
	}
	pr := mergedBelow(lower)
	pr.Health.Base = "main;evil"
	if err := f.svc.restack(ctx, lower.ID, pr); err != nil {
		t.Fatal(err)
	}
	if got := fromGitHub(t, f, upper.ID); len(got) != 1 || !strings.Contains(got[0], "you decide") {
		t.Errorf("the side track's relay: %q", got)
	}
}

// A release PR from the repo's base doesn't restack the side tracks on
// that base.
func TestAMergeFromTheBaseRestacksNothing(t *testing.T) {
	ctx := context.Background()
	f, main, lower, upper := stackedFixture(t)
	f.svc.GitHub = &fakeGitHub{}
	pr := mergedBelow(main)
	pr.Health.HeadBranch, pr.Health.Base = "main", "release"
	if err := f.svc.restack(ctx, main.ID, pr); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, lower.ID); len(got) != 0 {
		t.Errorf("an unstacked side track was restacked: %q", got)
	}
	_ = upper
}
