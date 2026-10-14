package tracks

import (
	"context"
	"slices"
	"testing"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/track"
)

func TestCountCommentsAndRunAction(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	gh := &fakeGitHub{}
	f.svc.GitHub = gh
	created, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id, window := created.Track.ID, created.Window.ID
	const open, merged = "https://github.com/acme/api/pull/1", "https://github.com/acme/api/pull/2"
	if err := f.svc.SeePRs(ctx, id, []string{open, merged}); err != nil {
		t.Fatal(err)
	}
	done, _ := track.ParsePR(merged)
	done.State = track.PRMerged
	if _, err := f.store.SavePR(ctx, id, done, f.svc.now()); err != nil {
		t.Fatal(err)
	}

	run := func() (string, error) { return f.svc.RunAction(ctx, id, actions.PRComments) }
	if _, err := run(); err == nil {
		t.Error("sent with no comments counted")
	}

	gh.threads = map[string]int{open: 3, merged: 5}
	if err := f.svc.CountComments(ctx); err != nil {
		t.Fatal(err)
	}
	if want := []string{"threads " + open}; !slices.Equal(gh.asked, want) {
		t.Errorf("asked %q, want %q: a merged PR's threads don't count", gh.asked, want)
	}
	if got := f.windows.comments[window]; got != (actions.Comments{Open: 3}) {
		t.Errorf("the window shows %+v, want 3 open", got)
	}

	if _, err := f.svc.RunAction(ctx, id, "nope"); err == nil {
		t.Error("ran an unknown action")
	}
	if err := f.svc.Report(ctx, id, track.AgentWaiting); err != nil {
		t.Fatal(err)
	}
	if _, err := run(); err == nil || len(f.windows.prompts) != 0 {
		t.Errorf("typed into an agent waiting on a dialog: %v, %q", err, f.windows.prompts)
	}
	if err := f.svc.Report(ctx, id, track.AgentWorking); err != nil {
		t.Fatal(err)
	}

	if name, err := run(); err != nil || name != created.Track.Name {
		t.Fatalf("RunAction = %s, %v", name, err)
	}
	want, _ := actions.Prompt(actions.PRComments, "claude")
	if !slices.Equal(f.windows.prompts, []string{window + " " + want}) {
		t.Errorf("typed %q, want the Claude prompt into %s", f.windows.prompts, window)
	}
	if got := f.windows.comments[window]; got != (actions.Comments{Open: 3, Sent: true}) {
		t.Errorf("after sending, the window shows %+v", got)
	}
	if _, err := run(); err == nil || len(f.windows.prompts) != 1 {
		t.Error("sent the same comments twice")
	}

	gh.threads[open] = 4
	if err := f.svc.CountComments(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.windows.comments[window]; got != (actions.Comments{Open: 4}) {
		t.Errorf("after a new comment, the window shows %+v, want 4 to send", got)
	}
}
