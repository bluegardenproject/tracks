package tracks

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// ReviewerPanes runs a track's reviewers, each in a train's session of
// its own, which the user sees in its overlay.
type ReviewerPanes interface {
	// OpenReviewer starts s as a track's reviewer.
	OpenReviewer(s trackwin.TrainSpec) error
	// CloseReviewer ends track's reviewer handle and its session.
	CloseReviewer(track, handle string) error
	// Reviewer is track's reviewer handle: its pane, "" for none, and
	// whether an overlay is attached to it.
	Reviewer(track, handle string) (ReviewerState, error)
	// Reviewers are every track's reviewers, by track, read at once,
	// without whether an overlay is attached.
	Reviewers() (map[string][]ReviewerState, error)
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

// ReviewerState is a track's reviewer: its handle, its pane, "" for
// none, and its session; whether an overlay is attached to it, and
// whether its agent exited, its pane left.
type ReviewerState struct {
	Handle, Pane, Session string
	Attached, Exited      bool
}

// Started is the reviewer StartReview started: its handle, the agent
// and model it runs on, for people, and the checkout it reviews.
type Started struct {
	Handle, Name, Target string
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

// StartReview starts a reviewer of req's diff in a train's session of track
// req.ID, on the agent and model the settings pick. It ends the reviewer
// of the same target that's there, a new review starting fresh, and
// leaves the others running.
func (s *Service) StartReview(ctx context.Context, req ReviewRequest) (Started, error) {
	if s.Reviewers == nil || s.HooksDir == "" {
		return Started{}, errors.New("reviews need tmux and a hooks folder")
	}
	t, err := s.Store.Track(ctx, req.ID)
	if errors.Is(err, store.ErrNotFound) {
		return Started{}, Problem("That track is gone.")
	} else if err != nil {
		return Started{}, err
	}
	if req.Packet.Empty() {
		return Started{}, Problem("There are no changes to review.")
	}
	worktree, ok := "", filepath.IsAbs(req.Dir)
	if ok {
		worktree, ok = ownWorktree(t, req.Dir)
	}
	if !ok {
		return Started{}, Problem("Run tracks review in one of the track's worktrees.")
	}
	if req.Packet, err = s.addPR(ctx, t, req.Packet, req.Stale); err != nil {
		return Started{}, err
	}
	set, err := s.Settings()
	if err != nil {
		return Started{}, fmt.Errorf("the reviewer setting can't be read: %w", err)
	}
	id, model := set.ReviewerOn(t.Engine)
	e, ok := agents.ByID(id)
	if !ok {
		return Started{}, Problem(fmt.Sprintf("Tracks doesn't know the reviewer's agent %q.", id))
	}
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	handle, err := s.replaceReviewer(ctx, t.ID, worktree, set.Agents.ReviewerPrefix())
	if err != nil {
		return Started{}, err
	}
	dir := s.trainDir(t.ID, handle)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Started{}, err
	}
	// A reviewer that didn't start leaves no folder: one would pass
	// for a reviewer to --wait and --ask.
	opened := false
	defer func() {
		if !opened {
			_ = os.RemoveAll(dir)
		}
	}()
	run := newRun()
	brief := filepath.Join(dir, "brief.md")
	if err := os.WriteFile(brief, []byte(review.Brief(claude.ReviewerInstructions(), t.Candor, req.Packet.Fit())), 0o600); err != nil {
		return Started{}, err
	}
	for name, text := range map[string]string{"agent": string(train.Reviewer), "target": worktree, "run": run} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			return Started{}, err
		}
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
			return Started{}, err
		}
		script = string(claude.ReviewerLine(e.Program, model, hookFile, brief, review.PromptGiven))
	case agents.Cursor.ID:
		engine, ok := s.engine(id)
		if !ok {
			return Started{}, Problem(fmt.Sprintf("Tracks doesn't know the reviewer's agent %q.", id))
		}
		chat, err := engine.Session(ctx, e.Program)
		if err != nil {
			return Started{}, fmt.Errorf("start the reviewer's chat: %w", err)
		}
		sess.Chat = chat
		script = sess.cursorTurn(review.Prompt(brief), filepath.Join(dir, "report.out"), false)
	default:
		return Started{}, Problem(e.Name + " can't review yet.")
	}
	if err := sess.save(dir); err != nil {
		return Started{}, err
	}
	name := e.Name
	if model != "" {
		name += " (" + model + ")"
	}
	spec := trackwin.TrainSpec{Track: t.ID, Handle: handle, Role: trackwin.RoleReviewer, Dir: worktree,
		Process: trackwin.Process{Title: "reviewer: " + name, Command: s.reviewerCommand(t.ID, script)},
		Header:  actions.TrainHeader(handle, train.Reviewer.Name(), trackwin.TrainBadgeOption)}
	if err := s.Reviewers.OpenReviewer(spec); err != nil {
		return Started{}, err
	}
	opened = true
	s.showBadge(trackwin.TrainSession(t.ID, handle), actions.Badges(track.Active))
	// Kept once it runs, as joined is posted: a left notice goes with
	// the handle, so the two pair up.
	if err := os.WriteFile(filepath.Join(dir, "handle"), []byte(handle), 0o600); err != nil {
		return Started{}, err
	}
	if err := s.notice(context.WithoutCancel(ctx), t.ID, "@"+handle+" joined: Reviewer on "+name+", reviewing the change."); err != nil {
		return Started{}, err
	}
	return Started{Handle: handle, Name: name, Target: worktree}, nil
}

// replaceReviewer ends track id's reviewers of target, saying so on its
// relay, and returns a handle for the new one: one no reviewer of the
// track has, nor had, the ended ones included. The caller holds
// s.reviewMu.
func (s *Service) replaceReviewer(ctx context.Context, id, target, prefix string) (string, error) {
	states, err := s.trainStates(id, "")
	if err != nil {
		return "", err
	}
	taken := map[string]bool{}
	for _, st := range states {
		taken[st.Handle] = true
		if st.Agent != train.Reviewer || !samePath(reviewTarget(st.Dir), target) {
			continue
		}
		if err := s.Reviewers.CloseReviewer(id, st.Handle); err != nil {
			return "", fmt.Errorf("end the reviewer that's there: %w", err)
		}
		if err := s.leftNotice(ctx, id, st.Dir, "a new review replaces it"); err != nil {
			return "", err
		}
		if err := os.RemoveAll(st.Dir); err != nil {
			return "", err
		}
	}
	if panes, err := s.Reviewers.Reviewers(); err == nil {
		for _, r := range panes[id] {
			taken[r.Handle] = true
		}
	}
	return newReviewHandle(prefix, func(h string) bool { return taken[h] })
}

// samePath says whether a and b are the same folder.
func samePath(a, b string) bool { return a != "" && resolvedPath(a) == resolvedPath(b) }

// ReviewOwner is the Code owner of dir's repo, dir a worktree of track
// id: what a review narrows to when asked for the code owner's part
// with no handle; "" when the repo has none.
func (s *Service) ReviewOwner(ctx context.Context, id, dir string) (string, error) {
	t, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return "", Problem("That track is gone.")
	} else if err != nil {
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

// ReviewReply takes a reply of one of track id's reviewers, the one on
// run, from its hook: the one that ends a review is its report, the
// first after it while a question of AskReview waits is its answer,
// and the others are the user's talk with it in its pane. A reply of
// another run, from a reviewer since ended, is dropped.
func (s *Service) ReviewReply(ctx context.Context, id, run, text string) error {
	if _, err := s.Store.Track(ctx, id); err != nil {
		return err
	}
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	st, ok, err := s.trainByRun(id, run)
	if err != nil || !ok {
		return err
	}
	dir := st.Dir
	path := filepath.Join(dir, "report.md")
	if _, err := os.Stat(path); err == nil {
		if !asking(dir) {
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

// WaitReview waits for the report of the current review of track id's
// reviewer to, or, once it was asked a question, for its answer to the
// last one. to is a handle or a prefix only one has; "" when the track
// has only one reviewer.
func (s *Service) WaitReview(ctx context.Context, id, to string) (string, error) {
	t, err := s.Store.Track(ctx, id)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, reviewWait)
	defer cancel()
	// Picked under the lock, so an AskReview on its way is seen whole.
	s.reviewMu.Lock()
	st, err := s.pickReviewer(t, to)
	if err != nil {
		s.reviewMu.Unlock()
		return "", err
	}
	path, what := filepath.Join(st.Dir, "report.md"), "report"
	if exists(filepath.Join(st.Dir, "question.md")) {
		path, what = filepath.Join(st.Dir, "answer.md"), "answer"
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
		if readTrim(filepath.Join(st.Dir, "run")) != st.Run {
			return "", Problem("@" + st.Handle + " was ended or replaced: tracks train list lists the reviewers that run.")
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return "", Problem(fmt.Sprintf("No %s after %s. tracks train list says whether @%s still "+
					"runs or its agent exited; the user can look at it in its overlay (tracks train show %s).",
					what, reviewWait, st.Handle, st.Handle))
			}
			return "", ctx.Err()
		case <-time.After(reviewPoll):
		}
	}
}

// liveReviews are track id's reviews with a run and a pane, as Trains
// lists them: a folder whose pane was closed some other way is no
// reviewer to wait for or ask. The caller holds s.reviewMu.
func (s *Service) liveReviews(id string) ([]trainState, error) {
	states, err := s.trainStates(id, train.Reviewer)
	if err != nil {
		return nil, err
	}
	var panes map[string][]ReviewerState
	if s.Reviewers != nil {
		if panes, err = s.Reviewers.Reviewers(); err != nil {
			return nil, err
		}
	}
	var out []trainState
	for _, st := range states {
		if st.Run != "" && (s.Reviewers == nil || slices.ContainsFunc(panes[id], func(r ReviewerState) bool { return r.Handle == st.Handle })) {
			out = append(out, st)
		}
	}
	return out, nil
}

// pickReviewer is track t's reviewer to, a handle or a prefix only one
// has; with to "", its only one. Several and no to is a Problem that
// lists them, for the caller to name one. The caller holds s.reviewMu.
func (s *Service) pickReviewer(t track.Track, to string) (trainState, error) {
	states, err := s.liveReviews(t.ID)
	if err != nil {
		return trainState{}, err
	}
	switch {
	case len(states) == 0:
		return trainState{}, Problem("This track has no review running: start one with tracks review.")
	case to == "" && len(states) == 1:
		return states[0], nil
	case to == "":
		names := make([]string, len(states))
		for i, st := range states {
			names[i] = "@" + st.Handle + " on " + reviewTarget(st.Dir)
		}
		return trainState{}, Problem(fmt.Sprintf("%s has %d reviewers: %s. Name one with --to <handle>.",
			t.Name, len(states), strings.Join(names, ", ")))
	}
	handles := make([]string, len(states))
	for i, st := range states {
		handles[i] = st.Handle
	}
	h, err := train.Resolve(to, handles)
	if err != nil {
		return trainState{}, Problem(fmt.Sprintf("%v: tracks train list lists the reviewers.", err))
	}
	return states[slices.IndexFunc(states, func(st trainState) bool { return st.Handle == h })], nil
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

// CloseReviewer ends track id's reviewer handle and closes its
// session: what End train does. A tracks review --wait on it stops,
// since its run is gone. It returns the track's name.
func (s *Service) CloseReviewer(ctx context.Context, id, handle string) (string, error) {
	if s.Reviewers == nil {
		return "", errors.New("reviews need tmux")
	}
	t, err := s.Store.Track(ctx, id)
	if err != nil {
		return "", err
	}
	if train.CheckHandle(handle) != nil {
		return t.Name, Problem(fmt.Sprintf("%q isn't a reviewer of %s.", handle, t.Name))
	}
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	st, ok, err := s.trainByHandle(id, handle)
	if err != nil {
		return t.Name, err
	}
	// The run goes whatever tmux says: the user asked for the review
	// to end.
	closeErr := s.Reviewers.CloseReviewer(id, handle)
	if !ok {
		return t.Name, closeErr
	}
	return t.Name, errors.Join(closeErr, s.leftNotice(ctx, id, st.Dir, "it was ended"), os.RemoveAll(st.Dir))
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
