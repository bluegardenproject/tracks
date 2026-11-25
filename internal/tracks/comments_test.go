package tracks

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/settings"
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
	const first, second, merged = "https://github.com/acme/api/pull/1", "https://github.com/acme/api/pull/2", "https://github.com/acme/api/pull/3"
	if err := f.svc.SeePRs(ctx, id, []string{first, second, merged}); err != nil {
		t.Fatal(err)
	}
	done, _ := track.ParsePR(merged)
	done.State = track.PRMerged
	if _, err := f.store.SavePR(ctx, id, done, f.svc.now()); err != nil {
		t.Fatal(err)
	}
	count := func() {
		t.Helper()
		gh.asked = nil
		if err := f.svc.PollPRs(ctx); err != nil {
			t.Fatal(err)
		}
		if err := f.svc.CountComments(ctx); err != nil {
			t.Fatal(err)
		}
	}
	run := func(button string) error {
		_, err := f.svc.RunAction(ctx, id, button)
		return err
	}
	// signals are the PRs Signals shows, as "PR: action (sent)".
	signals := func() string {
		t.Helper()
		sig, err := f.svc.Signals(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, pr := range sig.PRs {
			line := fmt.Sprintf("%d %s: %s", pr.Index, pr.PR, pr.Action)
			if pr.Sent {
				line += " (sent)"
			}
			out = append(out, line)
		}
		return strings.Join(out, "; ")
	}

	if err := run(actions.Range(0)); err == nil {
		t.Error("sent with no comments counted")
	}

	gh.threads = map[string]int{first: 3, second: 1, merged: 5}
	count()
	if slices.Contains(gh.asked, "view "+merged) || !slices.Contains(gh.asked, "view "+first) || !slices.Contains(gh.asked, "view "+second) {
		t.Errorf("asked %q: want the open PRs, and not the merged one", gh.asked)
	}
	if got, want := signals(), "0 PR #1: solve 3 comments; 1 PR #2: solve 1 comment"; got != want {
		t.Errorf("Signals shows %q, want %q", got, want)
	}
	if buttons := f.windows.buttons[window]; strings.Contains(buttons, "pr:") {
		t.Errorf("the PRs' buttons stayed in the footer: %s", buttons)
	}

	if err := run("nope"); err == nil {
		t.Error("ran an unknown button")
	}
	if err := run(actions.Range(9)); err == nil {
		t.Error("ran the button of a PR the track hasn't got")
	}
	if err := f.svc.Report(ctx, id, track.AgentWaiting); err != nil {
		t.Fatal(err)
	}
	if err := run(actions.Range(1)); err == nil || len(f.windows.prompts) != 0 {
		t.Errorf("typed into an agent waiting on a dialog: %v, %q", err, f.windows.prompts)
	}
	if err := f.svc.Report(ctx, id, track.AgentWorking); err != nil {
		t.Fatal(err)
	}

	if err := run(actions.Range(1)); err != nil {
		t.Fatal(err)
	}
	want, _ := actions.Prompt("claude", second, true)
	if !slices.Equal(f.windows.prompts, []string{window + " " + want}) {
		t.Errorf("typed %q, want the prompt for PR 2 into %s", f.windows.prompts, window)
	}
	if got, want := signals(), "0 PR #1: solve 3 comments; 1 PR #2: sent to the agent (sent)"; got != want {
		t.Errorf("after sending PR 2: %q, want %q", got, want)
	}
	if err := run(actions.Range(1)); err == nil || len(f.windows.prompts) != 1 {
		t.Error("sent the same comments twice")
	}
	off := false
	f.svc.Settings = func() (settings.Settings, error) {
		return settings.Settings{GitHub: settings.GitHub{AskBeforeHumanReplies: &off}}, nil
	}
	if err := run(actions.Range(0)); err != nil {
		t.Fatal(err)
	}
	if own, _ := actions.Prompt("claude", first, false); len(f.windows.prompts) != 2 || f.windows.prompts[1] != window+" "+own {
		t.Errorf("typed %q; want PR 1's prompt replying to people on its own", f.windows.prompts)
	}

	gh.threads[second] = 2
	count()
	if got, want := signals(), "0 PR #1: sent to the agent (sent); 1 PR #2: solve 2 comments"; got != want {
		t.Errorf("after a new comment on PR 2: %q, want %q", got, want)
	}
	f.svc.Settings = func() (settings.Settings, error) { return settings.Settings{}, errors.New("yaml: line 3: bad") }
	if err := run(actions.Range(1)); err != nil {
		t.Fatal(err)
	}
	if ask, _ := actions.Prompt("claude", second, true); len(f.windows.prompts) != 3 || f.windows.prompts[2] != window+" "+ask {
		t.Errorf("typed %q; want PR 2's prompt asking first, settings.yaml not read", f.windows.prompts)
	}
	gh.threads = map[string]int{}
	count()
	if got := signals(); got != "" {
		t.Errorf("with every comment answered, Signals shows %q", got)
	}
}

func TestPRSignalsAcrossRepos(t *testing.T) {
	var tr track.Track
	for _, url := range []string{"https://github.com/acme/predictions-live-app/pull/23", "https://github.com/acme/tracks/pull/12"} {
		pr, _ := track.ParsePR(url)
		tr.PRs = append(tr.PRs, pr)
	}
	got := prSignals(tr, map[string]actions.Comments{tr.PRs[0].URL: {Open: 4}, tr.PRs[1].URL: {Open: 1}})
	if len(got) != 2 || got[0].PR != "predictions-li…#23" || got[0].Action != "solve 4 comments" || got[1].PR != "tracks#12" || got[1].Action != "solve 1 comment" {
		t.Errorf("with PRs in two repos, want the repos named: %+v", got)
	}
}
