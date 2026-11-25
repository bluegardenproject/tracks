package tracks

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/track"
)

// fromGitHub are @github's messages on track id's relay, as "kind: body".
func fromGitHub(t *testing.T, f *fixture, id string) []string {
	t.Helper()
	log, err := f.store.RelayLog(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range log {
		if m.From == relay.GitHubHandle {
			out = append(out, string(m.Kind)+": "+m.Body)
		}
	}
	return out
}

func waitingFor(t *testing.T, f *fixture, id string) []string {
	t.Helper()
	all, err := f.store.WaitingInstructions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, in := range all {
		if in.TrackID == id {
			out = append(out, in.Kind+": "+in.Body)
		}
	}
	return out
}

func TestSideTracksGitHubEvents(t *testing.T) {
	ctx := context.Background()
	pr, _ := track.ParsePR("https://github.com/acme/api/pull/231")
	pr.Health = track.Health{Base: "main", Head: "0123456789", Checks: track.ChecksFailing, Failing: []string{"lint"}}
	conflicts := ghEvent{Kind: settings.EventConflicts, PR: pr, Body: "PR acme/api#231 has conflicts with main."}
	ci := ghEvent{Kind: settings.EventCIFailed, PR: pr, Body: "CI failed on PR acme/api#231 at 0123456: lint."}
	threads := ghEvent{Kind: settings.EventReviewThreads, PR: pr, Threads: 3, Body: "3 review threads on PR acme/api#231 wait for an answer."}
	merged := ghEvent{Kind: eventMerged, PR: pr, Body: "PR acme/api#231 was merged into main."}

	t.Run("defaults", func(t *testing.T) {
		f, main, st := sideFixture(t)
		for _, e := range []ghEvent{conflicts, ci, threads, merged} {
			if err := f.svc.githubEvent(ctx, st, e); err != nil {
				t.Fatal(err)
			}
		}
		if got := fromGitHub(t, f, st.ID); len(got) != 0 {
			t.Errorf("the side track heard the facts itself: %q", got)
		}
		named := "@" + st.Side + ", side track relay-store: "
		want := []string{
			"notice: " + named + conflicts.Body + " Tracks tells it to handle it, once its agent is free.",
			"notice: " + named + ci.Body + " Tracks tells it to handle it, once its agent is free.",
			"event: " + named + threads.Body,
			"event: " + named + merged.Body,
		}
		if got := fromGitHub(t, f, main.ID); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("the main track's relay:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
		want = []string{
			"conflicts: " + actions.SideConflicts("acme/api#231", "main"),
			"ci_failed: " + actions.SideCIFailed("acme/api#231", "0123456", "lint"),
		}
		if got := waitingFor(t, f, st.ID); strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("waiting for the side track: %q, want %q", got, want)
		}
	})

	t.Run("ask me, and handle threads automatically", func(t *testing.T) {
		f, main, st := sideFixture(t)
		f.svc.Settings = func() (settings.Settings, error) {
			return settings.Settings{GitHub: settings.GitHub{Conflicts: settings.PolicyAsk, ReviewThreads: settings.PolicyAuto}}, nil
		}
		for _, e := range []ghEvent{conflicts, threads} {
			if err := f.svc.githubEvent(ctx, st, e); err != nil {
				t.Fatal(err)
			}
		}
		if got := waitingFor(t, f, main.ID); len(got) != 1 || got[0] != "ask:conflicts: "+actions.AskMain(st.Side, "relay-store", conflicts.Body) {
			t.Errorf("waiting for the main track: %q; want it to ask the user", got)
		}
		work, _ := actions.SideThreads("claude", "acme/api#231", pr.URL, 3)
		if got := waitingFor(t, f, st.ID); len(got) != 1 || got[0] != "review_threads: "+work {
			t.Errorf("waiting for the side track: %q; want the threads' prompt", got)
		}
	})

	t.Run("not a side track, or its main track ended", func(t *testing.T) {
		f, main, st := sideFixture(t)
		if err := f.svc.githubEvent(ctx, main, conflicts); err != nil {
			t.Fatal(err)
		}
		if got := fromGitHub(t, f, main.ID); len(got) != 1 || got[0] != "event: "+conflicts.Body {
			t.Errorf("a track that isn't a side track: %q; want the event, as before", got)
		}
		if err := f.store.SetState(ctx, main.ID, main.State.Apply(track.Ended, f.svc.now())); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.githubEvent(ctx, st, conflicts); err != nil {
			t.Fatal(err)
		}
		if got := fromGitHub(t, f, st.ID); len(got) != 1 || got[0] != "event: "+conflicts.Body {
			t.Errorf("a side track whose main track ended: %q; want the event itself", got)
		}
	})
}

func TestInstructionsAreTypedOnceTheAgentIsFree(t *testing.T) {
	ctx := context.Background()
	f, _, st := sideFixture(t)
	if err := f.svc.instruct(ctx, st.ID, settings.EventConflicts, "pr", "Rebase."); err != nil {
		t.Fatal(err)
	}
	window := ""
	for _, w := range f.windows.windows {
		if w.Track == st.ID {
			window = w.Window
		}
	}
	typed := func() []string {
		t.Helper()
		before := len(f.windows.pasted)
		if err := f.svc.Deliver(ctx); err != nil {
			t.Fatal(err)
		}
		return f.windows.pasted[before:]
	}
	f.svc.SetTurn(ctx, st.ID, hooks.TurnStart)
	if got := typed(); len(got) != 0 {
		t.Fatalf("typed into an agent in a turn: %q", got)
	}
	f.svc.SetTurn(ctx, st.ID, hooks.TurnEnd)
	if got := typed(); len(got) != 1 || got[0] != window+" Rebase." {
		t.Fatalf("typed %q; want the instruction into %s", got, window)
	}
	if got := waitingFor(t, f, st.ID); len(got) != 0 {
		t.Errorf("still waiting: %q", got)
	}
	if !f.svc.inTurn(st.ID) {
		t.Error("the instruction starts the agent's turn, which holds the next back")
	}
	f.svc.SetTurn(ctx, st.ID, hooks.TurnEnd)
	if got := typed(); len(got) != 0 {
		t.Errorf("typed again: %q", got)
	}
}

func TestAskInstructionsAreKeptApart(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	f.svc.Settings = func() (settings.Settings, error) {
		return settings.Settings{GitHub: settings.GitHub{Conflicts: settings.PolicyAsk, CIFailed: settings.PolicyAsk}}, nil
	}
	pr, _ := track.ParsePR("https://github.com/acme/api/pull/231")
	for _, e := range []ghEvent{
		{Kind: settings.EventConflicts, PR: pr, Body: "conflicts"},
		{Kind: settings.EventCIFailed, PR: pr, Body: "CI"},
	} {
		if err := f.svc.githubEvent(ctx, st, e); err != nil {
			t.Fatal(err)
		}
	}
	if got := waitingFor(t, f, main.ID); len(got) != 2 {
		t.Errorf("waiting for the main track: %q; want both questions, one of the same PR's not replacing the other", got)
	}
}

func TestMootInstructionsAreDropped(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	f.svc.Settings = func() (settings.Settings, error) {
		return settings.Settings{GitHub: settings.GitHub{CIFailed: settings.PolicyAsk}}, nil
	}
	pr, _ := track.ParsePR("https://github.com/acme/api/pull/231")
	for _, e := range []ghEvent{
		{Kind: settings.EventConflicts, PR: pr, Body: "conflicts"},
		{Kind: settings.EventCIFailed, PR: pr, Body: "CI"},
	} {
		if err := f.svc.githubEvent(ctx, st, e); err != nil {
			t.Fatal(err)
		}
	}
	pending := pr
	pending.Health = track.Health{Mergeable: track.MergeConflicts, Checks: track.ChecksPending}
	if err := f.svc.dropDone(ctx, pending); err != nil {
		t.Fatal(err)
	}
	if side, asks := waitingFor(t, f, st.ID), waitingFor(t, f, main.ID); len(side) != 1 || len(asks) != 0 {
		t.Errorf("after CI started again: side %q, main %q; want the conflicts' only", side, asks)
	}
	clean := pr
	clean.Health = track.Health{Mergeable: track.MergeClean, Checks: track.ChecksPassing}
	if err := f.svc.dropDone(ctx, clean); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, st.ID); len(got) != 0 {
		t.Errorf("after the conflicts were solved: %q", got)
	}

	if err := f.svc.instruct(ctx, st.ID, settings.EventReviewThreads, pr.URL, "threads"); err != nil {
		t.Fatal(err)
	}
	merged := pr
	merged.State = track.PRMerged
	if err := f.svc.dropDone(ctx, merged); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, st.ID); len(got) != 0 {
		t.Errorf("after the PR merged: %q", got)
	}
}

func TestAnEndedSideTracksEventsGoToItsMainTrack(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	if err := f.store.SetState(ctx, st.ID, st.State.Apply(track.Ended, f.svc.now())); err != nil {
		t.Fatal(err)
	}
	ended, err := f.store.Track(ctx, st.ID)
	if err != nil {
		t.Fatal(err)
	}
	pr, _ := track.ParsePR("https://github.com/acme/api/pull/231")
	if err := f.svc.githubEvent(ctx, ended, ghEvent{Kind: settings.EventConflicts, PR: pr, Body: "PR acme/api#231 has conflicts with main."}); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, st.ID); len(got) != 0 {
		t.Errorf("instructed an ended side track: %q", got)
	}
	if got := fromGitHub(t, f, main.ID); len(got) != 1 || !strings.HasPrefix(got[0], "event: @"+st.Side) {
		t.Errorf("the main track's relay: %q; want the event, for it to decide", got)
	}
	f.svc.Settings = func() (settings.Settings, error) {
		return settings.Settings{GitHub: settings.GitHub{CIFailed: settings.PolicyAsk}}, nil
	}
	if err := f.svc.githubEvent(ctx, ended, ghEvent{Kind: settings.EventCIFailed, PR: pr, Body: "CI failed."}); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, main.ID); len(got) != 0 {
		t.Errorf("asked about an ended side track: %q; want its main track to decide", got)
	}
}

// An ended side track's open PR still counts its review threads, for
// its main track.
func TestAnEndedSideTracksThreadsGoToItsMainTrack(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	gh := &fakeGitHub{}
	f.svc.GitHub = gh
	const url = "https://github.com/acme/api/pull/231"
	if err := f.svc.SeePRs(ctx, st.ID, []string{url}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.SetState(ctx, st.ID, st.State.Apply(track.Ended, f.svc.now())); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 2} {
		gh.threads = map[string]int{url: n}
		if err := f.svc.PollPRs(ctx); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.CountComments(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got := fromGitHub(t, f, main.ID); len(got) != 1 || !strings.Contains(got[0], "2 review threads") {
		t.Errorf("the main track's relay: %q; want the new threads", got)
	}
}

// An instruction goes before a due pointer, and marks the review
// threads' Signals button sent; a pointer waits while it's pasted.
func TestAnInstructionGoesFirst(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	const url = "https://github.com/acme/api/pull/1"
	if err := f.svc.SeePRs(ctx, "a", []string{url}); err != nil {
		t.Fatal(err)
	}
	f.svc.mu.Lock()
	f.svc.comments = map[string]map[string]actions.Comments{"a": {url: {Open: 2}}}
	f.svc.mu.Unlock()
	post(t, f, relay.Message{From: "reviewer-1", To: "track", Kind: relay.Report, Body: "Fine.", At: *now})
	*now = now.Add(pointerGrace)
	if err := f.svc.instruct(ctx, "a", settings.EventReviewThreads, url, "Work through them."); err != nil {
		t.Fatal(err)
	}
	if got := deliver(t, f); len(got) != 1 || got[0] != "@a Work through them." {
		t.Fatalf("pasted %q; want the instruction alone", got)
	}
	f.svc.mu.Lock()
	sent := f.svc.comments["a"][url].Sent
	f.svc.mu.Unlock()
	if !sent {
		t.Error("the review threads' button isn't marked sent")
	}
	if got := deliver(t, f); len(got) != 0 {
		t.Errorf("pasted %q while the instruction waits for its turn", got)
	}
	f.svc.SetTurn(ctx, "a", hooks.TurnStart)
	f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
	if got := deliver(t, f); len(got) != 1 || !strings.Contains(got[0], "on the relay") {
		t.Errorf("after its turn pasted %q; want the pointer", got)
	}
}

// An instruction the agent didn't take, its turn never starting,
// stalls as a pointer does, and frees the track.
func TestAnInstructionNotTakenStalls(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	if err := f.svc.instruct(ctx, "a", settings.EventConflicts, "pr", "Rebase."); err != nil {
		t.Fatal(err)
	}
	if got := deliver(t, f); len(got) != 1 {
		t.Fatalf("pasted %q; want the instruction", got)
	}
	*now = now.Add(pointerStall)
	if got := deliver(t, f); len(got) != 0 {
		t.Fatalf("pasted %q at the stall", got)
	}
	f.svc.mu.Lock()
	_, waiting := f.svc.pointing["a"]
	in, known := f.svc.turns["a"]
	f.svc.mu.Unlock()
	if waiting || !known || in {
		t.Errorf("after the stall: pointing %v, turn %v known %v; want the track free", waiting, in, known)
	}
}

// A side track that ends hands what waited for it to its main track.
func TestAnEndingSideTrackHandsOverItsInstructions(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	const url = "https://github.com/acme/api/pull/231"
	if err := f.svc.SeePRs(ctx, st.ID, []string{url}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.instruct(ctx, st.ID, settings.EventConflicts, url, "Rebase."); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.instruct(ctx, main.ID, askKind(settings.EventCIFailed), url, "Ask."); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Report(ctx, st.ID, track.Ended); err != nil {
		t.Fatal(err)
	}
	if side, asks := waitingFor(t, f, st.ID), waitingFor(t, f, main.ID); len(side)+len(asks) != 0 {
		t.Errorf("still waiting: side %q, main %q", side, asks)
	}
	got := fromGitHub(t, f, main.ID)
	if len(got) != 2 || !strings.Contains(got[0], "ended before Tracks' word about PR acme/api#231 went out") {
		t.Errorf("the main track's relay: %q; want an event for each", got)
	}
}

// Answered threads, or the user's click on Signals' button, drop a
// waiting threads instruction; one dropped meanwhile isn't pasted.
func TestThreadsInstructionsGoWhenMoot(t *testing.T) {
	ctx := context.Background()
	f, now := deliveryFixture(t)
	gh := &fakeGitHub{}
	f.svc.GitHub = gh
	const url = "https://github.com/acme/api/pull/1"
	if err := f.svc.SeePRs(ctx, "a", []string{url}); err != nil {
		t.Fatal(err)
	}
	count := func(n int) {
		t.Helper()
		gh.threads = map[string]int{url: n}
		if err := f.svc.PollPRs(ctx); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.CountComments(ctx); err != nil {
			t.Fatal(err)
		}
	}
	count(2)
	if err := f.svc.instruct(ctx, "a", settings.EventReviewThreads, url, "Work through them."); err != nil {
		t.Fatal(err)
	}
	count(0)
	if got := waitingFor(t, f, "a"); len(got) != 0 {
		t.Errorf("with every thread answered, still waiting: %q", got)
	}

	count(2)
	if err := f.svc.instruct(ctx, "a", settings.EventReviewThreads, url, "Work through them."); err != nil {
		t.Fatal(err)
	}
	f.svc.SetTurn(ctx, "a", hooks.TurnEnd)
	if _, err := f.svc.RunAction(ctx, "a", actions.Range(0)); err != nil {
		t.Fatal(err)
	}
	if got := waitingFor(t, f, "a"); len(got) != 0 {
		t.Errorf("after the user sent them from Signals, still waiting: %q", got)
	}

	if err := f.svc.instruct(ctx, "a", settings.EventConflicts, url, "Rebase."); err != nil {
		t.Fatal(err)
	}
	waiting, _ := f.store.WaitingInstructions(ctx)
	if err := f.store.DropInstructions(ctx, url, settings.EventConflicts); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	if typed, err := f.svc.typeInstruction(ctx, track.Track{ID: "a"}, "@a", waiting[0]); err != nil || typed {
		t.Errorf("pasted one dropped meanwhile: %v, %v", typed, err)
	}
}
