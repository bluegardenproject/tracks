package daemon

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
)

func TestNextLiveStatus(t *testing.T) {
	cases := []struct {
		name    string
		current state.Status
		idle    bool
		newPR   bool
		want    state.Status
	}{
		// PR URL detection takes priority over everything else.
		{"running + new PR → PR", state.StatusRunning, false, true, state.StatusPROpen},
		{"waiting + new PR → PR", state.StatusWaiting, false, true, state.StatusPROpen},
		{"PR + no new PR → still PR", state.StatusPROpen, false, false, state.StatusPROpen},

		// Idle heuristic: only flips Running↔Waiting.
		{"running + idle → waiting", state.StatusRunning, true, false, state.StatusWaiting},
		{"waiting + active → running", state.StatusWaiting, false, false, state.StatusRunning},

		// StatusPROpen is not overridden by the idle heuristic.
		{"PR + idle → still PR", state.StatusPROpen, true, false, state.StatusPROpen},
		{"PR + active → still PR", state.StatusPROpen, false, false, state.StatusPROpen},

		// A second PR on a track already in review keeps it there.
		{"PR + another new PR → still PR", state.StatusPROpen, false, true, state.StatusPROpen},

		// No transition when state already matches.
		{"running + active → running", state.StatusRunning, false, false, state.StatusRunning},
		{"waiting + idle → waiting", state.StatusWaiting, true, false, state.StatusWaiting},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := nextLiveStatus(c.current, c.idle, c.newPR); got != c.want {
				t.Errorf("nextLiveStatus(%q, idle=%v, newPR=%v) = %q, want %q",
					c.current, c.idle, c.newPR, got, c.want)
			}
		})
	}
}

// TestRefreshRunningStatusSetsPR verifies that a live track transitions to
// StatusPROpen (not StatusWaiting) when a new PR URL first appears while the
// pane is idle, and that the URL is persisted on the track.
func TestRefreshRunningStatusSetsPR(t *testing.T) {
	srv := newQuietServer(t)

	const prURL = "https://github.com/example/repo/pull/42"
	if err := srv.store.Put(state.Track{
		ID:     "t-pr",
		Status: state.StatusRunning,
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Simulate what refreshRunningStatus does inside store.Update when a
	// PR URL first appears: an unknown URL → target becomes StatusPROpen.
	_, _, _ = srv.store.Update("t-pr", func(t *state.Track) bool {
		added := unknownPRs(*t, []string{prURL})
		t.Status = nextLiveStatus(t.Status, true /* idle */, len(added) > 0)
		for _, url := range added {
			t.AddPR(url)
		}
		return true
	})

	got, ok := srv.store.Get("t-pr")
	if !ok {
		t.Fatal("track missing")
	}
	if got.Status != state.StatusPROpen {
		t.Errorf("status = %q, want %q", got.Status, state.StatusPROpen)
	}
	if len(got.PRs) != 1 || got.PRs[0].URL != prURL {
		t.Errorf("PRs = %+v, want one entry for %q", got.PRs, prURL)
	}
}

// TestRefreshRunningStatusPRNotOverriddenByIdle verifies that once a track
// is in StatusPROpen, subsequent idle polls do not flip it to StatusWaiting.
func TestRefreshRunningStatusPRNotOverriddenByIdle(t *testing.T) {
	srv := newQuietServer(t)

	if err := srv.store.Put(state.Track{
		ID:     "t-pr2",
		Status: state.StatusPROpen,
		PRs:    []state.PRRef{{URL: "https://github.com/example/repo/pull/7", State: "OPEN"}},
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Simulate several idle polls after the PR is already set.
	for range 5 {
		_, _, _ = srv.store.Update("t-pr2", func(t *state.Track) bool {
			// URL already stored — not a new PR.
			newPR := len(unknownPRs(*t, []string{"https://github.com/example/repo/pull/7"})) > 0
			t.Status = nextLiveStatus(t.Status, true /* idle */, newPR)
			return true
		})
	}

	got, _ := srv.store.Get("t-pr2")
	if got.Status != state.StatusPROpen {
		t.Errorf("status after idle polls = %q, want %q", got.Status, state.StatusPROpen)
	}
}

// A track that opens several PRs collects one entry per marker, and the
// pane re-showing an already-known URL must not re-add it.
func TestUnknownPRs(t *testing.T) {
	trk := state.Track{PRs: []state.PRRef{{URL: "https://example.test/pr/1"}}}
	got := unknownPRs(trk, []string{
		"https://example.test/pr/1",
		"https://example.test/pr/2",
		"https://example.test/pr/3",
	})
	want := []string{"https://example.test/pr/2", "https://example.test/pr/3"}
	if len(got) != len(want) {
		t.Fatalf("unknownPRs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("unknownPRs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// terminalStatusFor is what decides whether a finished track reads as
// "done" or as a shipped PR.
func TestTerminalStatusFor(t *testing.T) {
	cases := []struct {
		name string
		trk  state.Track
		want state.Status
	}{
		{"no PR", state.Track{}, state.StatusDone},
		{"one merged", state.Track{PRs: []state.PRRef{{State: "MERGED"}}}, state.StatusPRMerged},
		{"one closed unmerged", state.Track{PRs: []state.PRRef{{State: "CLOSED"}}}, state.StatusDone},
		{"still open (ended by hand)", state.Track{PRs: []state.PRRef{{State: "OPEN"}}}, state.StatusDone},
		{"all merged", state.Track{PRs: []state.PRRef{{State: "MERGED"}, {State: "MERGED"}}}, state.StatusPRMerged},
		{"one of two closed", state.Track{PRs: []state.PRRef{{State: "MERGED"}, {State: "CLOSED"}}}, state.StatusDone},
	}
	for _, c := range cases {
		if got := terminalStatusFor(c.trk); got != c.want {
			t.Errorf("%s: terminalStatusFor() = %q, want %q", c.name, got, c.want)
		}
	}
}
