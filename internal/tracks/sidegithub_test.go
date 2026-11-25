package tracks

import (
	"context"
	"strings"
	"testing"

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
		if got := waitingFor(t, f, main.ID); len(got) != 1 || got[0] != "ask: "+actions.AskMain(st.Side, "relay-store", conflicts.Body) {
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
		before := len(f.windows.prompts)
		if err := f.svc.Deliver(ctx); err != nil {
			t.Fatal(err)
		}
		return f.windows.prompts[before:]
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
