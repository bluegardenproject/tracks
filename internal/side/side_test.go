package side

import (
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestSettled(t *testing.T) {
	ended := track.State{ClosedAt: time.UnixMilli(1)}
	pr := func(n int, s track.PRState) track.PR { return track.PR{Number: n, State: s} }
	for _, tc := range []struct {
		name string
		t    track.Track
		want bool
	}{
		{"running", track.Track{}, false},
		{"running, its PR merged", track.Track{PRs: []track.PR{pr(1, track.PRMerged)}}, false},
		{"ended without a PR", track.Track{State: ended}, true},
		{"ended, its PR open", track.Track{State: ended, PRs: []track.PR{pr(1, track.PROpen)}}, false},
		{"ended, its PR a draft", track.Track{State: ended, PRs: []track.PR{pr(1, track.PRDraft)}}, false},
		{"ended, its PRs merged and closed", track.Track{State: ended, PRs: []track.PR{pr(1, track.PRMerged), pr(2, track.PRClosed)}}, true},
	} {
		if got := Settled(tc.t); got != tc.want {
			t.Errorf("%s: settled %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestWaits(t *testing.T) {
	ended := track.State{ClosedAt: time.UnixMilli(1)}
	sides := []track.Track{
		{Name: "relay-store"},
		{Name: "relay-cli", State: ended, PRs: []track.PR{{Number: 231, State: track.PROpen}, {Number: 232, State: track.PRDraft}}},
		{Name: "docs", State: ended},
		{Name: "ui", PRs: []track.PR{{Number: 7, State: track.PROpen}}},
	}
	got := Waits("api-cache", "it ends", Left(sides))
	want := "api-cache waits for its side tracks before it ends: relay-store (running), relay-cli (PRs #231, #232 open), " +
		"ui (running, PR #7 open). End each, and merge or close its pull request."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	if got := Waits("api-cache", "it ends", Left(sides[2:3])); got != "" {
		t.Errorf("settled side tracks: %q, want nothing", got)
	}
}

func TestIs(t *testing.T) {
	if Is(track.Track{Parent: "a"}) || Is(track.Track{Side: "side-000001"}) || !Is(track.Track{Parent: "a", Side: "side-000001"}) {
		t.Error("a side track is one with a handle on the relay of a main track that's still there")
	}
}

func TestNotices(t *testing.T) {
	st := track.Track{Name: "relay-store", Side: "side-000123", Prompt: "  Store the relay\nin SQLite"}
	if got := Joined(st); got != "@side-000123 joined: side track relay-store, Store the relay" {
		t.Errorf("joined %q", got)
	}
	if got := LeftNotice(st, "it ended"); got != "@side-000123 left: it ended." {
		t.Errorf("left %q", got)
	}
}
