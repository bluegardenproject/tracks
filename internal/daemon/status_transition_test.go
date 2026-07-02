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
		{"running + new PR → PR", state.StatusRunning, false, true, state.StatusPR},
		{"waiting + new PR → PR", state.StatusWaiting, false, true, state.StatusPR},
		{"PR + no new PR → still PR", state.StatusPR, false, false, state.StatusPR},

		// Idle heuristic: only flips Running↔Waiting.
		{"running + idle → waiting", state.StatusRunning, true, false, state.StatusWaiting},
		{"waiting + active → running", state.StatusWaiting, false, false, state.StatusRunning},

		// StatusPR is not overridden by the idle heuristic.
		{"PR + idle → still PR", state.StatusPR, true, false, state.StatusPR},
		{"PR + active → still PR", state.StatusPR, false, false, state.StatusPR},

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
// StatusPR (not StatusWaiting) when a new PR URL first appears while the
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
	// PR URL first appears: newPR is true → target becomes StatusPR.
	_, _, _ = srv.store.Update("t-pr", func(t *state.Track) bool {
		newPR := prURL != "" && prURL != t.PRURL
		t.Status = nextLiveStatus(t.Status, true /* idle */, newPR)
		t.PRURL = prURL
		return true
	})

	got, ok := srv.store.Get("t-pr")
	if !ok {
		t.Fatal("track missing")
	}
	if got.Status != state.StatusPR {
		t.Errorf("status = %q, want %q", got.Status, state.StatusPR)
	}
	if got.PRURL != prURL {
		t.Errorf("PRURL = %q, want %q", got.PRURL, prURL)
	}
}

// TestRefreshRunningStatusPRNotOverriddenByIdle verifies that once a track
// is in StatusPR, subsequent idle polls do not flip it to StatusWaiting.
func TestRefreshRunningStatusPRNotOverriddenByIdle(t *testing.T) {
	srv := newQuietServer(t)

	if err := srv.store.Put(state.Track{
		ID:     "t-pr2",
		Status: state.StatusPR,
		PRURL:  "https://github.com/example/repo/pull/7",
	}); err != nil {
		t.Fatalf("put: %v", err)
	}

	// Simulate several idle polls after the PR is already set.
	for range 5 {
		_, _, _ = srv.store.Update("t-pr2", func(t *state.Track) bool {
			newPR := false // URL already stored — not a new PR
			t.Status = nextLiveStatus(t.Status, true /* idle */, newPR)
			return true
		})
	}

	got, _ := srv.store.Get("t-pr2")
	if got.Status != state.StatusPR {
		t.Errorf("status after idle polls = %q, want %q", got.Status, state.StatusPR)
	}
}
