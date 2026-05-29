package daemon

import (
	"context"
	"time"

	"github.com/bluegardenproject/tracks/internal/github"
	"github.com/bluegardenproject/tracks/internal/notify"
	"github.com/bluegardenproject/tracks/internal/state"
)

// prPollInterval is how often we ask GitHub for fresh state on a
// known PR. 60s is a deliberate middle ground: short enough to
// feel live when a reviewer leaves a comment, long enough that
// even 10 active PRs stay well inside gh's rate limit.
const prPollInterval = 60 * time.Second

// startPRWatcher launches one goroutine per track once the
// supervisor sees a PR URL appear on the track. The watcher polls
// `gh pr view` until the PR is merged/closed or the track ends
// (sup.done closes).
func (s *Server) startPRWatcher(sup *supervisor, url string) {
	s.mu.Lock()
	if sup.prWatcherStarted {
		s.mu.Unlock()
		return
	}
	sup.prWatcherStarted = true
	s.mu.Unlock()

	go s.runPRWatcher(sup, url)
}

func (s *Server) runPRWatcher(sup *supervisor, url string) {
	// First poll fires immediately so the dashboard reflects PR
	// state within a second of the URL appearing.
	if s.refreshPR(sup, url) {
		return
	}
	ticker := time.NewTicker(prPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-sup.done:
			return
		case <-ticker.C:
			if s.refreshPR(sup, url) {
				return
			}
		}
	}
}

// refreshPR polls gh once and updates state.Track. Returns true
// when the PR has reached a terminal state (MERGED/CLOSED) and the
// caller should stop polling.
func (s *Server) refreshPR(sup *supervisor, url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	status, err := github.Inspect(ctx, url)
	if err != nil {
		// Swallow — gh might be down or PR not yet visible. We'll
		// retry on the next tick.
		return false
	}

	t, ok := s.store.Get(sup.trackID)
	if !ok {
		return true // track is gone; stop polling.
	}
	prev := state.Track{
		PRState:       t.PRState,
		PRDraft:       t.PRDraft,
		PRReviewState: t.PRReviewState,
		PRComments:    t.PRComments,
	}
	t.PRState = status.State
	t.PRDraft = status.Draft
	t.PRReviewState = status.ReviewState
	t.PRComments = status.CommentCount
	_ = s.store.Put(t)

	// Notify only on review-decision changes — the user wants to
	// know "the PR needs me again" without being woken up by
	// every passing comment.
	if status.ReviewState != prev.PRReviewState && status.ReviewState != "" {
		s.notifyEvent(string(notify.EventPRStateChanged),
			"tracks: PR review update",
			labelFor(t)+" → "+humanReview(status.ReviewState))
	}
	if status.State != prev.PRState && status.State != "" && status.State != "OPEN" {
		s.notifyEvent(string(notify.EventPRStateChanged),
			"tracks: PR closed",
			labelFor(t)+" → "+humanState(status.State))
	}

	return status.State == "MERGED" || status.State == "CLOSED"
}

func humanReview(s string) string {
	switch s {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "changes requested"
	case "REVIEW_REQUIRED":
		return "review requested"
	default:
		return s
	}
}

func humanState(s string) string {
	switch s {
	case "OPEN":
		return "open"
	case "MERGED":
		return "merged"
	case "CLOSED":
		return "closed"
	default:
		return s
	}
}
