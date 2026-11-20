package tracks

import (
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/agents/claude"
)

// showAsking shows track id's reviewer r in window when it waits on
// Claude's question whether to trust the worktree, which no one sees in
// a hidden pane, and returns r as it then is. A run is shown for it once,
// once it's been seen open with the question, so a user who hides it
// again keeps it hidden; a show that failed is tried again on the next
// tick. Only a Claude reviewer still on its review is looked at.
func (s *Service) showAsking(id, window string, r ReviewerState) ReviewerState {
	if r.Pane == "" || s.HooksDir == "" {
		return r
	}
	dir := s.reviewDir(id)
	run, err := os.ReadFile(filepath.Join(dir, "run"))
	if err != nil || exists(filepath.Join(dir, "report.md")) {
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
