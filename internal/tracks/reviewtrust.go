package tracks

import (
	"os"
	"path/filepath"
	"time"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/agents/claude"
)

// trustWindow is how long after a review starts its reviewer is looked
// at for Claude's trust question, which Claude asks as it starts, before
// anything else: after that, a reviewer costs the tick nothing.
const trustWindow = time.Minute

// showAsking shows track id's reviewer r in window when it waits on
// Claude's question whether to trust the worktree, which no one sees in
// a hidden pane, and returns r as it then is. A run is shown for it once,
// once it's been seen open with the question, so a user who hides it
// again keeps it hidden; a show that failed is tried again on the next
// tick. Only a Claude reviewer in its first trustWindow is looked at.
func (s *Service) showAsking(id, window string, r ReviewerState) ReviewerState {
	if r.Pane == "" || s.HooksDir == "" {
		return r
	}
	dir := s.reviewDir(id)
	info, err := os.Stat(filepath.Join(dir, "run"))
	// A file's time is the real clock's, whatever s.Now says.
	if err != nil || time.Since(info.ModTime()) > trustWindow {
		return r
	}
	run, err := os.ReadFile(filepath.Join(dir, "run"))
	if err != nil {
		return r
	}
	s.mu.Lock()
	done := s.trustShown[id] == string(run)
	s.mu.Unlock()
	if done {
		return r
	}
	if sess, err := loadSession(dir); err != nil || sess.Engine != agents.Claude.ID {
		return r
	}
	screen, err := s.Reviewers.CaptureReviewer(r.Pane)
	if err != nil || !claude.AsksTrust(screen) {
		return r
	}
	if !r.Shown {
		if s.Reviewers.ShowReviewer(window, r.Pane) != nil {
			return r
		}
		r.Shown = true
	}
	s.mu.Lock()
	if s.trustShown == nil {
		s.trustShown = map[string]string{}
	}
	s.trustShown[id] = string(run)
	s.mu.Unlock()
	return r
}
