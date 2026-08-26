package daemon

import (
	"strconv"
	"sync"
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
)

// newWindowTestServer points the server at a tmux session name no real
// session will be using, so claimWindowName's listing finds nothing and
// the tests never reach the developer's own "tracks" session — which
// config.Default() would otherwise hand them.
func newWindowTestServer(t *testing.T) *Server {
	t.Helper()
	srv := newReadinessTestServer(t)
	cfg := srv.config()
	cfg.Tmux.SessionName = testSessionName(t)
	srv.cfg.Store(&cfg)
	return srv
}

func TestClaimWindowNamePlainWhenFree(t *testing.T) {
	srv := newWindowTestServer(t)
	got := srv.claimWindowName(state.Track{ID: "20260818-101530-aaaaaa", Slug: "swap tooltip"})
	if got != "swap-tooltip" {
		t.Errorf("name = %q, want the bare label", got)
	}
}

// The whole reason the id used to be stapled on: two tracks sharing a
// label must not share a window, or ending one kills the other.
func TestClaimWindowNameDisambiguatesAgainstAnotherTrack(t *testing.T) {
	srv := newWindowTestServer(t)
	if err := srv.store.Put(state.Track{
		ID: "20260818-100000-bbbbbb", Status: state.StatusRunning,
		Slug: "swap tooltip", Window: "swap-tooltip",
	}); err != nil {
		t.Fatal(err)
	}

	got := srv.claimWindowName(state.Track{ID: "20260818-101530-aaaaaa", Slug: "swap tooltip"})
	if got != "swap-tooltip-2" {
		t.Errorf("name = %q, want swap-tooltip-2", got)
	}
}

func TestClaimWindowNameCountsUpPastSeveral(t *testing.T) {
	srv := newWindowTestServer(t)
	for i, name := range []string{"swap-tooltip", "swap-tooltip-2", "swap-tooltip-3"} {
		if err := srv.store.Put(state.Track{
			ID: string(rune('a'+i)) + "-track", Status: state.StatusRunning, Window: name,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := srv.claimWindowName(state.Track{ID: "new", Slug: "swap tooltip"}); got != "swap-tooltip-4" {
		t.Errorf("name = %q, want swap-tooltip-4", got)
	}
}

// A draft has never been launched, so it owns no window and must not
// push a new track onto a suffix.
func TestClaimWindowNameIgnoresDrafts(t *testing.T) {
	srv := newWindowTestServer(t)
	if err := srv.store.Put(state.Track{
		ID: "draft", Status: state.StatusDraft, Slug: "swap tooltip", Window: "swap-tooltip",
	}); err != nil {
		t.Fatal(err)
	}
	if got := srv.claimWindowName(state.Track{ID: "new", Slug: "swap tooltip"}); got != "swap-tooltip" {
		t.Errorf("name = %q, want the plain label — a draft holds no window", got)
	}
}

// A track with neither slug nor prompt has no label to make readable,
// so it keeps the id-suffixed form, which is unique by construction.
func TestClaimWindowNameFallsBackWithoutALabel(t *testing.T) {
	srv := newWindowTestServer(t)
	got := srv.claimWindowName(state.Track{ID: "20260818-101530-aaaaaa"})
	if got != "t-aaaaaa" {
		t.Errorf("name = %q, want the id-suffixed fallback", got)
	}
}

// A finished-but-not-closed track still has its pane open as a shell,
// so its name is still taken.
func TestClaimWindowNameRespectsFinishedTracks(t *testing.T) {
	srv := newWindowTestServer(t)
	if err := srv.store.Put(state.Track{
		ID: "old", Status: state.StatusDone, Window: "swap-tooltip",
	}); err != nil {
		t.Fatal(err)
	}
	if got := srv.claimWindowName(state.Track{ID: "new", Slug: "swap tooltip"}); got != "swap-tooltip-2" {
		t.Errorf("name = %q, want swap-tooltip-2 — a done track keeps its window until it is closed", got)
	}
}

// The reservation is the point: handleNew does minutes of work between
// picking a name and persisting the track, so two overlapping creations
// must not both be handed the plain label.
func TestClaimWindowNameReservesUntilReleased(t *testing.T) {
	srv := newWindowTestServer(t)
	tr := state.Track{ID: "a", Slug: "swap tooltip"}

	first := srv.claimWindowName(tr)
	if first != "swap-tooltip" {
		t.Fatalf("first = %q, want the bare label", first)
	}
	// Nothing has been persisted yet — exactly the in-flight window.
	second := srv.claimWindowName(state.Track{ID: "b", Slug: "swap tooltip"})
	if second == first {
		t.Errorf("second creation got %q, the same name as the in-flight first", second)
	}
	if second != "swap-tooltip-2" {
		t.Errorf("second = %q, want swap-tooltip-2", second)
	}

	// Released (the track is in the store by then, so the store half of
	// the check takes over).
	srv.releaseWindowName(first)
	if got := srv.claimWindowName(state.Track{ID: "c", Slug: "swap tooltip"}); got != "swap-tooltip" {
		t.Errorf("after release = %q, want the freed name reusable", got)
	}
}

// Concurrent creations must never be handed the same name. Run under -race.
func TestClaimWindowNameIsUniqueUnderConcurrency(t *testing.T) {
	srv := newWindowTestServer(t)

	const n = 12
	names := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			names[i] = srv.claimWindowName(state.Track{ID: strconv.Itoa(i), Slug: "swap tooltip"})
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, n)
	for _, name := range names {
		if seen[name] {
			t.Errorf("name %q handed out twice — two tracks would share a window", name)
		}
		seen[name] = true
	}
}

// A track with no label takes the id-suffixed form and reserves nothing,
// so releasing it is a no-op rather than a panic.
func TestReleaseWindowNameIsSafeForUnreserved(t *testing.T) {
	srv := newWindowTestServer(t)
	name := srv.claimWindowName(state.Track{ID: "20260818-101530-aaaaaa"})
	srv.releaseWindowName(name)
	srv.releaseWindowName(name)
}
