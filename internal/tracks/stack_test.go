package tracks

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/actions"
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
	if err := f.svc.prHealth(ctx, lower.ID, pr, track.PROpen, nil); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, upper.ID); len(got) != 0 {
		t.Errorf("restacked before the poll saved every PR: %q", got)
	}
	if err := f.svc.restackMerged(ctx); err != nil {
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
