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
// `gh pr view` for every PR the track has opened — re-reading the list
// each tick, so PRs opened later are picked up without a second
// goroutine — until none are open any more or the track ends (sup.done
// closes).
func (s *Server) startPRWatcher(sup *supervisor) {
	s.mu.Lock()
	if sup.prWatcherStarted {
		s.mu.Unlock()
		return
	}
	sup.prWatcherStarted = true
	s.mu.Unlock()

	go s.runPRWatcher(sup)
}

func (s *Server) runPRWatcher(sup *supervisor) {
	// First poll fires immediately so the dashboard reflects PR
	// state within a second of the URL appearing.
	if s.refreshPRs(sup) && s.onPRTerminal(sup) {
		return
	}
	ticker := time.NewTicker(prPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-sup.done:
			return
		case <-ticker.C:
			terminal := s.refreshPRs(sup)
			// A track in review keeps accruing usage if the user resumes
			// Claude to address comments — keep the stored figure current.
			s.refreshUsage(sup)
			if terminal && s.onPRTerminal(sup) {
				return
			}
		}
	}
}

// onPRTerminal is called once every PR the track opened is merged or
// closed, and reports whether the watcher is finished.
//
// It finalizes the track only when Claude has already exited — that's a
// track sitting in review whose last PR just landed. If Claude is still
// running (it opened a PR early and kept working, which is exactly the
// stacked-PR flow) the track must stay live: it may open more PRs, and
// finalizing here would stamp an end state on a live session, after
// which every state write is refused as terminal. In that case the
// watcher keeps ticking so those later PRs are picked up too, and the
// normal Claude-exit path owns the end-state transition.
func (s *Server) onPRTerminal(sup *supervisor) bool {
	t, ok := s.store.Get(sup.trackID)
	if !ok {
		return true // track is gone.
	}
	if t.Status != state.StatusPROpen {
		return true // someone else already settled it.
	}
	if !sup.claudeExited() {
		return false
	}
	s.retire(sup)
	return true
}

// refreshPRs polls gh once for each of the track's pull requests and
// updates their stored state. Returns true when the caller should stop
// polling: the track is gone, has no PRs, or none of its PRs is open any
// more.
func (s *Server) refreshPRs(sup *supervisor) bool {
	t, ok := s.store.Get(sup.trackID)
	if !ok {
		return true // track is gone; stop polling.
	}
	if len(t.PRs) == 0 {
		return true
	}
	for _, pr := range t.PRs {
		// Skip PRs already merged or closed. A closed PR can technically be
		// reopened, but polling a settled stack once a minute for that is
		// not worth the gh calls — the user reopening one can reopen the
		// track too.
		if !pr.Open() {
			continue
		}
		if !s.refreshPR(sup.trackID, pr.URL) {
			return true // track vanished mid-poll.
		}
	}
	updated, ok := s.store.Get(sup.trackID)
	return !ok || !updated.HasOpenPR()
}

// refreshPR polls gh once for one PR and updates its entry on the
// track. Returns false only when the track no longer exists.
func (s *Server) refreshPR(trackID, url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	status, err := github.Inspect(ctx, url)
	if err != nil {
		// Swallow — gh might be down or PR not yet visible. We'll
		// retry on the next tick.
		return true
	}

	var prev state.PRRef
	var known bool
	updated, found, _ := s.store.Update(trackID, func(t *state.Track) bool {
		i := t.PRIndex(url)
		if i < 0 {
			return false
		}
		known = true
		prev = t.PRs[i]
		next := prev
		next.State = status.State
		next.Draft = status.Draft
		next.ReviewState = status.ReviewState
		next.Comments = status.CommentCount
		// False when nothing changed, which skips the write + flush.
		return t.SetPR(i, next)
	})
	if !found {
		return false
	}
	if !known {
		// The track was replaced under us (a Forget + New reusing the ID)
		// and no longer carries this PR. prev is meaningless, so notifying
		// off it would report a transition that never happened.
		return true
	}
	t := updated

	// Notify only on review-decision changes — the user wants to
	// know "the PR needs me again" without being woken up by
	// every passing comment. The PR number is included when the track
	// carries several, so the user knows which one moved.
	suffix := ""
	if len(t.PRs) > 1 {
		if n := prev.Number(); n != "" {
			suffix = " " + n
		}
	}
	if status.ReviewState != prev.ReviewState && status.ReviewState != "" {
		s.notifyEvent(string(notify.EventPRStateChanged),
			"tracks: PR review update",
			labelFor(t)+suffix+" → "+humanReview(status.ReviewState))
	}
	if status.State != prev.State && status.State != "" && status.State != "OPEN" {
		s.notifyEvent(string(notify.EventPRStateChanged),
			"tracks: PR closed",
			labelFor(t)+suffix+" → "+humanState(status.State))
	}

	return true
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
