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

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/agents/claude"
	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/train"
)

// ReviewerPanes runs a track's reviewer in a train's session of its
// own, which the user sees in its overlay.
type ReviewerPanes interface {
	// OpenReviewer starts s as a track's reviewer.
	OpenReviewer(s trackwin.TrainSpec) error
	// CloseReviewer ends track's reviewer and its session.
	CloseReviewer(track string) error
	// Reviewer is track's reviewer: its pane, "" for none, and whether
	// an overlay is attached to it.
	Reviewer(track string) (ReviewerState, error)
	// Reviewers are every track's reviewers, by track, read at once,
	// without whether an overlay is attached.
	Reviewers() (map[string]ReviewerState, error)
	// TrainBadge sets the badges in the header of the train in session,
	// a tmux format, and redraws its overlay's header.
	TrainBadge(session, badges string) error
	// TypeReviewer types text into the reviewer's pane and sends it.
	TypeReviewer(pane, text string) error
	// RespawnReviewer ends what the reviewer's pane runs and starts
	// command in dir there.
	RespawnReviewer(pane, dir, command string) error
	// CaptureReviewer is what the reviewer's pane shows.
	CaptureReviewer(pane string) (string, error)
}

// ReviewerState is a track's reviewer: its pane, "" for none, and its
// session; whether an overlay is attached to it, and whether its agent
// exited, its pane left.
type ReviewerState struct {
	Pane, Session    string
	Attached, Exited bool
}

// ReviewRequest is a review of the changes in Dir, a worktree of track
// ID. The caller gathers the packet and checks the worktree is
// unchanged after, in its own sandbox; the daemon runs no git, since a
// repo's config, which an agent can write, can make git run commands.
type ReviewRequest struct {
	ID     string        `json:"id"`
	Dir    string        `json:"dir"`
	Packet review.Packet `json:"packet"`
	// Stale reviews a pull request's worktree that's behind the PR.
	Stale bool `json:"stale,omitempty"`
}

// reviewPoll is how often WaitReview looks for the report.
const reviewPoll = 500 * time.Millisecond

// reviewDir holds track id's review: its brief, its run and its report.
// It sits with the track's hooks, which go when the track ends.
func (s *Service) reviewDir(id string) string { return filepath.Join(s.HooksDir, id, "review") }

// StartReview starts a reviewer of req's diff in a train's session of track
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
	if req.Packet.Empty() {
		return "", Problem("There are no changes to review.")
	}
	worktree, ok := "", filepath.IsAbs(req.Dir)
	if ok {
		worktree, ok = ownWorktree(t, req.Dir)
	}
	if !ok {
		return "", Problem("Run tracks review in one of the track's worktrees.")
	}
	if req.Packet, err = s.addPR(ctx, t, req.Packet, req.Stale); err != nil {
		return "", err
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
	dir := s.reviewDir(t.ID)
	oldHandle := reviewHandle(dir)
	if err := s.Reviewers.CloseReviewer(t.ID); err != nil {
		return "", fmt.Errorf("end the reviewer that's there: %w", err)
	}
	if err := s.leftNotice(ctx, t.ID, dir, "a new review replaces it"); err != nil {
		return "", err
	}

	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	handle, err := newReviewHandle(set.Agents.ReviewerPrefix(), oldHandle)
	if err != nil {
		return "", err
	}
	run := newRun()
	brief := filepath.Join(dir, "brief.md")
	if err := os.WriteFile(brief, []byte(review.Brief(claude.ReviewerInstructions(), t.Candor, req.Packet.Fit())), 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "run"), []byte(run), 0o600); err != nil {
		return "", err
	}
	reply := hooks.ReviewerCommand(filepath.Join(s.BinDir, "tracks"), id, t.ID, run)
	sess := reviewSession{Engine: id, Program: e.Program, Model: model, Workspace: worktree, Reply: reply,
		Tracks: filepath.Join(s.BinDir, "tracks")}
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
		sess.Chat = chat
		script = sess.cursorTurn(review.Prompt(brief), filepath.Join(dir, "report.out"), false)
	default:
		return "", Problem(e.Name + " can't review yet.")
	}
	if err := sess.save(dir); err != nil {
		return "", err
	}
	name := e.Name
	if model != "" {
		name += " (" + model + ")"
	}
	spec := trackwin.TrainSpec{Track: t.ID, Handle: handle, Role: trackwin.RoleReviewer, Dir: worktree,
		Process: trackwin.Process{Title: "reviewer: " + name, Command: s.reviewerCommand(t.ID, script)},
		Header:  actions.TrainHeader(handle, train.Reviewer.Name(), trackwin.TrainBadgeOption)}
	if err := s.Reviewers.OpenReviewer(spec); err != nil {
		return "", err
	}
	s.showBadge(trackwin.TrainSession(t.ID, handle), actions.Badges(track.Active))
	// Kept once it runs, as joined is posted: a left notice goes with
	// the handle, so the two pair up.
	if err := os.WriteFile(filepath.Join(dir, "handle"), []byte(handle), 0o600); err != nil {
		return "", err
	}
	if err := s.notice(context.WithoutCancel(ctx), t.ID, "@"+handle+" joined: Reviewer on "+name+", reviewing the change."); err != nil {
		return "", err
	}
	return name, nil
}

// ReviewOwner is the code owner a review of dir, a worktree of track
// id, narrows to by default: its repo's Code owner on a Review track,
// "" for the whole change. A Work track's pre-push review checks its own
// whole change.
func (s *Service) ReviewOwner(ctx context.Context, id, dir string) (string, error) {
	t, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return "", Problem("That track is gone.")
	} else if err != nil || t.Kind != track.Review {
		return "", err
	}
	worktree, ok := "", filepath.IsAbs(dir)
	if ok {
		worktree, ok = ownWorktree(t, dir)
	}
	if !ok {
		return "", Problem("Run tracks review in one of the track's worktrees.")
	}
	repos, err := s.Store.Repos(ctx)
	if err != nil {
		return "", err
	}
	for _, r := range t.Repos {
		if r.Worktree != worktree || r.RepoID == 0 {
			continue
		}
		for _, repo := range repos {
			if repo.ID == r.RepoID {
				return repo.CodeOwner, nil
			}
		}
	}
	return "", nil
}

// reviewerCommand runs script in the reviewer's pane: with the track's
// ID, Tracks on PATH and the review marker, and nothing after it. The
// pane stays when the reviewer exits, so it's listed as exited.
func (s *Service) reviewerCommand(id, script string) string {
	env := "TRACKS_ID=" + shellx.Quote(id) + " TRACKS_SOCKET_DIR=" + shellx.Quote(s.SocketDir) + " " + review.EnvMarker + "=1"
	if s.BinDir != "" {
		env += " PATH=" + shellx.Quote(s.BinDir) + `:"$PATH"`
	}
	return env + " sh -c " + shellx.Quote(script)
}

// ReviewReply takes a reply of track id's reviewer from its hook: the
// one that ends a review is its report, the first after it while a
// question of AskReview waits is its answer, and the others are the
// user's talk with it in its pane. A reply of another run, from a
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
	path := filepath.Join(dir, "report.md")
	if _, err := os.Stat(path); err == nil {
		if !s.asking(id) {
			return nil
		}
		path = filepath.Join(dir, "answer.md")
	} else if !strings.Contains(text, review.Outcome) {
		return nil
	}
	// Posted first, so the tracks review that prints it finds the
	// message to mark read.
	kind := relay.Report
	if filepath.Base(path) == "answer.md" {
		kind = relay.Answer
	}
	if err := s.postReview(ctx, id, dir, kind, text); err != nil {
		return err
	}
	return writeWhole(path, text)
}

// writeWhole writes text to path whole and then renames it, so
// WaitReview never reads half.
func writeWhole(path, text string) error {
	if err := os.WriteFile(path+".part", []byte(text), 0o600); err != nil {
		return err
	}
	return os.Rename(path+".part", path)
}

// reviewWait bounds WaitReview: a reviewer that never gives a report,
// stuck or asking something no one answers, doesn't hold its caller
// for good.
var reviewWait = 30 * time.Minute

// WaitReview waits for the report of track id's current review, or,
// once it was asked a question, for its answer to the last one.
func (s *Service) WaitReview(ctx context.Context, id string) (string, error) {
	if _, err := s.Store.Track(ctx, id); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, reviewWait)
	defer cancel()
	// Picked under the lock, so an AskReview on its way is seen whole.
	s.reviewMu.Lock()
	path, what := filepath.Join(s.reviewDir(id), "report.md"), "report"
	if exists(filepath.Join(s.reviewDir(id), "question.md")) {
		path, what = filepath.Join(s.reviewDir(id), "answer.md"), "answer"
	}
	s.reviewMu.Unlock()
	for {
		// The reply and its message's number are read together, as
		// ReviewReply writes them.
		s.reviewMu.Lock()
		b, err := os.ReadFile(path)
		msg := printedMessage(path)
		s.reviewMu.Unlock()
		if err == nil {
			if msg == 0 {
				return string(b), nil
			}
			return string(b), s.markReplyRead(ctx, id, msg)
		}
		if _, err := os.Stat(filepath.Join(s.reviewDir(id), "run")); err != nil {
			return "", Problem("This track has no review running: start one with tracks review.")
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", Problem(fmt.Sprintf("No %s after %s. tracks train list says whether the reviewer still "+
					"runs or its agent exited; the user can look at it in its overlay (tracks train show reviewer).", what, reviewWait))
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

// CloseReviewer ends track id's reviewer and closes its session: what
// End train does. A tracks review --wait on it stops, since its run is
// gone. It returns the track's name.
func (s *Service) CloseReviewer(ctx context.Context, id string) (string, error) {
	if s.Reviewers == nil {
		return "", errors.New("reviews need tmux")
	}
	t, err := s.Store.Track(ctx, id)
	if err != nil {
		return "", err
	}
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	// The run goes whatever tmux says: the user asked for the review
	// to end.
	closeErr := s.Reviewers.CloseReviewer(id)
	return t.Name, errors.Join(closeErr, s.leftNotice(ctx, id, s.reviewDir(id), "it was ended"), os.RemoveAll(s.reviewDir(id)))
}

// markReplyRead marks the reviewer's reply, message msg on track id's
// relay, read for the track, which got it printed. The track reads its
// relay with one cursor: one before an earlier message it hasn't read,
// an @github event come in meanwhile, stays there, so that one still
// wakes its agent, which reads the reply again with it.
func (s *Service) markReplyRead(ctx context.Context, id string, msg int64) error {
	unread, err := s.Store.RelayUnread(ctx, id, relay.TrackHandle)
	if err != nil {
		return err
	}
	upTo := msg
	for _, m := range unread {
		if m.ID < msg && m.Kind != relay.Notice {
			upTo = m.ID - 1
			break
		}
	}
	if upTo == 0 {
		return nil
	}
	_, err = s.Store.MarkRelayRead(ctx, id, relay.TrackHandle, upTo)
	return err
}
