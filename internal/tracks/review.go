package tracks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/agents/claude"
	"github.com/bluegardenproject/tracks/internal/agents/cursor"
	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// ReviewerPanes runs a track's reviewer in a pane of its own, hidden
// until the user shows it.
type ReviewerPanes interface {
	// OpenReviewer starts p in dir as track's reviewer, hidden.
	OpenReviewer(track, dir string, p trackwin.Process) error
	// CloseReviewer ends track's reviewer, shown in window or hidden.
	CloseReviewer(track, window string) error
}

// ReviewRequest is a review of the changes in Dir, a worktree of track
// ID. The caller makes the diff and checks the worktree is unchanged
// after, in its own sandbox; the daemon runs no git, since a repo's
// config, which an agent can write, can make git run commands.
type ReviewRequest struct {
	ID   string `json:"id"`
	Dir  string `json:"dir"`
	Diff string `json:"diff"`
}

// reviewPoll is how often WaitReview looks for the report.
const reviewPoll = 500 * time.Millisecond

// reviewDir holds track id's review: its brief, its run and its report.
// It sits with the track's hooks, which go when the track ends.
func (s *Service) reviewDir(id string) string { return filepath.Join(s.HooksDir, id, "review") }

// StartReview starts a reviewer of req's diff in a hidden pane of track
// req.ID, on the agent and model the settings pick, ending one that's
// there: a new review starts fresh. It returns the reviewer's name for
// people.
func (s *Service) StartReview(ctx context.Context, req ReviewRequest) (string, error) {
	if s.Reviewers == nil || s.HooksDir == "" {
		return "", errors.New("reviews need tmux and a hooks folder")
	}
	t, err := s.Store.Track(ctx, req.ID)
	if errors.Is(err, store.ErrNotFound) {
		return "", Problem("That track is gone.")
	} else if err != nil {
		return "", err
	}
	worktree, ok := "", filepath.IsAbs(req.Dir)
	if ok {
		worktree, ok = ownWorktree(t, req.Dir)
	}
	if !ok {
		return "", Problem("Run tracks review in one of the track's worktrees.")
	}
	set, err := s.Settings()
	if err != nil {
		return "", fmt.Errorf("the reviewer setting can't be read: %w", err)
	}
	id, model := set.ReviewerOn(t.Engine)
	e, ok := agents.ByID(id)
	if !ok {
		return "", Problem(fmt.Sprintf("Tracks doesn't know the reviewer's agent %q.", id))
	}
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	window, _ := s.windowOf(t.ID)
	if err := s.Reviewers.CloseReviewer(t.ID, window); err != nil {
		return "", fmt.Errorf("end the reviewer that's there: %w", err)
	}

	dir := s.reviewDir(t.ID)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	run := newRun()
	brief := filepath.Join(dir, "brief.md")
	if err := os.WriteFile(brief, []byte(review.Brief(claude.ReviewerInstructions(), t.Candor, req.Diff)), 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "run"), []byte(run), 0o600); err != nil {
		return "", err
	}
	reply := hooks.ReviewerCommand(filepath.Join(s.BinDir, "tracks"), id, t.ID, run)
	var script string
	switch id {
	case agents.Claude.ID:
		// Claude's Stop hook carries its reply.
		hookFile, err := hooks.Install(filepath.Join(dir, "hooks"), id, reply, s.Socket)
		if err != nil {
			return "", err
		}
		script = string(claude.ReviewerLine(e.Program, model, hookFile, brief, review.PromptGiven))
	case agents.Cursor.ID:
		engine, ok := s.engine(id)
		if !ok {
			return "", Problem(fmt.Sprintf("Tracks doesn't know the reviewer's agent %q.", id))
		}
		chat, err := engine.Session(ctx, e.Program)
		if err != nil {
			return "", fmt.Errorf("start the reviewer's chat: %w", err)
		}
		report := filepath.Join(dir, "report.out")
		script = cursor.Reviewer{
			Program: e.Program, Model: model, Chat: chat, Workspace: worktree, Prompt: review.Prompt(brief),
			Report: report, Reply: reply + " --reply-file " + shellx.Quote(report),
		}.Script()
	default:
		return "", Problem(e.Name + " can't review yet.")
	}
	name := e.Name
	if model != "" {
		name += " (" + model + ")"
	}
	p := trackwin.Process{Title: "reviewer: " + name, Command: s.reviewerCommand(t.ID, script)}
	if err := s.Reviewers.OpenReviewer(t.ID, worktree, p); err != nil {
		return "", err
	}
	return name, nil
}

// reviewerCommand runs script in the reviewer's pane: with the track's
// ID, Tracks on PATH and the review marker, and nothing after it, so
// the pane closes with the reviewer.
func (s *Service) reviewerCommand(id, script string) string {
	env := "TRACKS_ID=" + shellx.Quote(id) + " TRACKS_SOCKET_DIR=" + shellx.Quote(s.SocketDir) + " " + review.EnvMarker + "=1"
	if s.BinDir != "" {
		env += " PATH=" + shellx.Quote(s.BinDir) + `:"$PATH"`
	}
	return env + " sh -c " + shellx.Quote(script)
}

// ReviewReply takes a reply of track id's reviewer from its hook: the
// one that ends a review is its report. A reply of another run, from a
// reviewer since ended, is dropped.
func (s *Service) ReviewReply(ctx context.Context, id, run, text string) error {
	if _, err := s.Store.Track(ctx, id); err != nil {
		return err
	}
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	dir := s.reviewDir(id)
	current, err := os.ReadFile(filepath.Join(dir, "run"))
	if err != nil || string(current) != run {
		return nil
	}
	if !strings.Contains(text, review.Outcome) {
		return nil
	}
	path := filepath.Join(dir, "report.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	// Written whole and then renamed, so WaitReview never reads half.
	if err := os.WriteFile(path+".part", []byte(text), 0o600); err != nil {
		return err
	}
	return os.Rename(path+".part", path)
}

// reviewWait bounds WaitReview: a reviewer that never gives a report,
// stuck or asking something no one answers, doesn't hold its caller
// for good.
var reviewWait = 30 * time.Minute

// WaitReview waits for the report of track id's current review.
func (s *Service) WaitReview(ctx context.Context, id string) (string, error) {
	if _, err := s.Store.Track(ctx, id); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, reviewWait)
	defer cancel()
	path := filepath.Join(s.reviewDir(id), "report.md")
	for {
		if b, err := os.ReadFile(path); err == nil {
			return string(b), nil
		}
		if _, err := os.Stat(filepath.Join(s.reviewDir(id), "run")); err != nil {
			return "", Problem("This track has no review running: start one with tracks review.")
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", Problem(fmt.Sprintf("No report after %s. The reviewer still runs in its pane: "+
					"the footer's button shows it.", reviewWait))
			}
			return "", ctx.Err()
		case <-time.After(reviewPoll):
		}
	}
}

// newRun names a review run, so a reply of an ended reviewer is told
// from the current one's.
func newRun() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ownWorktree is the worktree of t that dir is in: the reviewer reads
// what's there, and only a track's own may be read. It compares paths,
// without git.
func ownWorktree(t track.Track, dir string) (string, bool) {
	d := resolvedPath(dir)
	for _, r := range t.Repos {
		if r.Worktree == "" {
			continue
		}
		w := resolvedPath(r.Worktree)
		if d == w || strings.HasPrefix(d, w+string(filepath.Separator)) {
			return r.Worktree, true
		}
	}
	return "", false
}

// resolvedPath is path, clean, with its links resolved where they can
// be.
func resolvedPath(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return filepath.Clean(path)
}
