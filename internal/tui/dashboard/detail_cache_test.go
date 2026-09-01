package dashboard

import (
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// detailModel builds a model sitting on tracks[0] with a detail panel
// already gathered for cachedID and good until goodUntil. Tracks carry
// no Repos, so gatherDetail shells out to nothing — these tests are
// about the caching decision, not about git.
func detailModel(track state.Track, cachedID string, goodUntil time.Time) *model {
	m := &model{tracks: []state.Track{track}}
	m.detail = &detail{track: state.Track{ID: cachedID}}
	m.detailID = cachedID
	m.detailGoodUntil = goodUntil
	return m
}

// brokenRepoModel is a model whose single track points at a repo path
// that doesn't exist, so every git call in gatherDetail errors.
func brokenRepoModel() *model {
	return &model{
		cfg: config.Config{Repos: []config.Repo{{Name: "r", Base: "main", Path: "/nonexistent"}}},
		tracks: []state.Track{{
			ID:    "a",
			Repos: []state.TrackRepo{{Name: "r", Path: "/nonexistent"}},
		}},
	}
}

func TestRefreshDetailIfStaleKeepsFreshGitHalves(t *testing.T) {
	until := time.Now().Add(detailTTL / 2)
	m := detailModel(state.Track{ID: "a"}, "a", until)
	m.detail.files = []string{"repo: M\tfoo.go"}
	before := m.detail

	m.refreshDetailIfStale()

	if !m.detailGoodUntil.Equal(until) {
		t.Errorf("re-gathered inside the TTL: goodUntil moved to %v", m.detailGoodUntil)
	}
	if m.detail != before || len(m.detail.files) != 1 {
		t.Error("re-gathered inside the TTL: the cached git halves were replaced")
	}
}

// The blocking bug this pins: only the git halves are cached. Status,
// idle, usage and the action hints are rendered off detail.track and
// must track the live record, or the panel disagrees with the table
// row directly above it for up to a minute.
func TestRefreshDetailIfStaleRepointsTrackOnACacheHit(t *testing.T) {
	m := detailModel(state.Track{ID: "a", Status: state.StatusRunning}, "a", time.Now().Add(detailTTL/2))
	m.detail.track = state.Track{ID: "a", Status: state.StatusInterrupted}

	m.refreshDetailIfStale()

	if m.detail.track.Status != state.StatusRunning {
		t.Errorf("panel served a stale track: status = %q, want %q",
			m.detail.track.Status, state.StatusRunning)
	}
}

func TestRefreshDetailIfStaleRegathersPastTTL(t *testing.T) {
	until := time.Now().Add(-time.Second)
	m := detailModel(state.Track{ID: "a"}, "a", until)

	m.refreshDetailIfStale()

	if !m.detailGoodUntil.After(until) {
		t.Error("did not re-gather an expired panel")
	}
}

// A cursor move is handled by refreshDetail directly, but the poll can
// also land on a different track (selection re-anchors by ID after
// tracks are added or forgotten). A stale ID must not be served from
// cache however recently it was gathered.
func TestRefreshDetailIfStaleRegathersOnADifferentTrack(t *testing.T) {
	m := detailModel(state.Track{ID: "b"}, "a", time.Now().Add(detailTTL))

	m.refreshDetailIfStale()

	if m.detailID != "b" {
		t.Errorf("served another track's cached panel: detailID = %q, want %q", m.detailID, "b")
	}
}

// refreshDetail is the cursor-move path: the user is asking for this
// row, so it must gather unconditionally rather than consult the TTL.
func TestRefreshDetailIgnoresTheTTL(t *testing.T) {
	// Twice the TTL, so a forced gather must move the deadline *down*
	// to now+detailTTL. Asserting a decrease keeps the test off any
	// dependence on the clock advancing between two Now() calls.
	until := time.Now().Add(2 * detailTTL)
	m := detailModel(state.Track{ID: "a"}, "a", until)

	m.refreshDetail()

	if !m.detailGoodUntil.Before(until) {
		t.Error("refreshDetail honoured the TTL instead of forcing a gather")
	}
}

// `r` is the only escape hatch from the TTL when the cursor is already
// on the row the user wants refreshed.
func TestInvalidateDetailForcesTheNextPollToRegather(t *testing.T) {
	m := detailModel(state.Track{ID: "a"}, "a", time.Now().Add(detailTTL))

	m.invalidateDetail()
	m.refreshDetailIfStale()

	if !m.detailGoodUntil.After(time.Now()) {
		t.Error("invalidateDetail did not cause the next poll to re-gather")
	}
}

// A gather that hit a git error is held for the short retry window,
// not the full TTL — its empty halves mean "couldn't read", not
// "nothing yet", and pinning that on screen for a minute is wrong.
//
// The repo path doesn't exist, so ExecRunner fails on chdir before it
// reaches git and gatherDetail flags the result incomplete.
func TestRefreshDetailRetriesAnIncompleteGatherSooner(t *testing.T) {
	m := brokenRepoModel()

	m.refreshDetail()

	if m.detail == nil || !m.detail.incomplete {
		t.Fatalf("expected an incomplete gather, got %+v", m.detail)
	}
	if m.detailGoodUntil.After(time.Now().Add(detailRetryTTL)) {
		t.Errorf("an incomplete gather was held past the retry window: %v", m.detailGoodUntil)
	}
}

// A repo that fails every time — a finished track whose worktree the
// daemon removed — must not re-spawn git every detailRetryTTL forever.
// Only the first failure earns the short window.
func TestRefreshDetailBacksOffAfterARepeatedFailure(t *testing.T) {
	m := brokenRepoModel()

	m.refreshDetail() // first failure: short retry
	m.refreshDetail() // still failing: full TTL

	if !m.detail.incomplete {
		t.Fatal("expected the second gather to fail too")
	}
	if !m.detailGoodUntil.After(time.Now().Add(detailRetryTTL)) {
		t.Errorf("a repeatedly-failing repo is still on the retry cadence: %v", m.detailGoodUntil)
	}
}

// A complete gather that simply found nothing — a new track with no
// commits and no edits — is not a failure and gets the full TTL.
func TestRefreshDetailHoldsAnEmptyButCompleteGather(t *testing.T) {
	m := &model{tracks: []state.Track{{ID: "a"}}}

	m.refreshDetail()

	if m.detail == nil || m.detail.incomplete {
		t.Fatalf("an empty gather should not be flagged incomplete: %+v", m.detail)
	}
	if !m.detailGoodUntil.After(time.Now().Add(detailRetryTTL)) {
		t.Error("an empty-but-complete gather should be held for the full TTL")
	}
}

func TestRefreshDetailIfStaleClearsWithNoTracks(t *testing.T) {
	m := detailModel(state.Track{ID: "a"}, "a", time.Now().Add(detailTTL))
	m.tracks = nil

	m.refreshDetailIfStale()

	if m.detail != nil || m.detailID != "" {
		t.Errorf("kept a panel with no tracks: detail = %v, detailID = %q", m.detail, m.detailID)
	}
}

// The cursor can sit past the end of a list that shrank between polls;
// the guard must clear rather than index out of range.
func TestRefreshDetailIfStaleClearsWithTheCursorPastTheEnd(t *testing.T) {
	m := detailModel(state.Track{ID: "a"}, "a", time.Now().Add(detailTTL))
	m.cursor = 5

	m.refreshDetailIfStale()

	if m.detail != nil || m.detailID != "" {
		t.Errorf("kept a panel with the cursor out of range: detail = %v, detailID = %q",
			m.detail, m.detailID)
	}
}
