package tracks

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/train"
)

// The reviewer runs no commands, so Tracks speaks for it on the relay:
// it posts its report and answers as its messages, types the track's
// questions into its session, and says when it joins and leaves.

// reviewHandle is the handle of the reviewer whose folder is dir: the
// folder's name, or for a review's folder from before trains had
// folders of their own, the handle it kept, the reviewer's prefix alone
// for an older one still.
func reviewHandle(dir string) string {
	if filepath.Base(filepath.Dir(dir)) == "trains" {
		return filepath.Base(dir)
	}
	if h := readTrim(filepath.Join(dir, "handle")); train.CheckHandle(h) == nil {
		return h
	}
	return train.ReviewerHandle
}

// reviewTarget is the checkout the reviewer whose folder is dir
// reviews: as it was kept, or, for a review from before, the worktree
// its session runs in.
func reviewTarget(dir string) string {
	if t := readTrim(filepath.Join(dir, "target")); t != "" {
		return t
	}
	sess, _ := loadSession(dir)
	return sess.Workspace
}

// newReviewHandle is a handle for a new reviewer that taken doesn't
// hold. prefix is the setting's, the reviewer's name when it's unset or
// isn't one.
func newReviewHandle(prefix string, taken func(string) bool) (string, error) {
	if train.CheckPrefix(prefix) != nil {
		prefix = train.Reviewer.Prefix()
	}
	return train.NewHandle(prefix, taken)
}

// postReview posts text, the report or an answer of track id's reviewer,
// as its message to the track, a report with its outcome, and keeps the
// message's number in dir, so the tracks review that prints it can mark
// it read. The caller holds s.reviewMu.
func (s *Service) postReview(ctx context.Context, id, dir string, kind relay.Kind, text string) error {
	m := relay.Message{From: reviewHandle(dir), To: relay.TrackHandle, Kind: kind, Body: text, At: s.now()}
	if kind == relay.Report {
		m.Outcome = reviewOutcome(text)
	}
	m, err := s.Store.AddRelayMessage(ctx, id, m)
	if err != nil {
		return err
	}
	if s.Notify != nil {
		if t, err := s.Store.Track(ctx, id); err == nil {
			s.notify(trainNotice(id, t.Name, m))
		}
	}
	return writeWhole(filepath.Join(dir, string(kind)+".msg"), strconv.FormatInt(m.ID, 10))
}

// reviewOutcome is the outcome of a reviewer's report, read from its
// REVIEW OUTCOME line; none for a report without one.
func reviewOutcome(report string) relay.Outcome {
	switch train.OutcomeOf(report) {
	case train.Pass:
		return relay.Pass
	case train.Blocked:
		return relay.Blocked
	}
	return ""
}

// printedMessage is the number of the relay message of the report or
// answer at path, which tracks review prints and then marks read; 0 for
// none. Marking is a reader's cursor, so what came before it on the
// relay counts as read too. The caller holds s.reviewMu.
func printedMessage(path string) int64 {
	kind := relay.Report
	if strings.HasPrefix(filepath.Base(path), "answer") {
		kind = relay.Answer
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(path), string(kind)+".msg"))
	if err != nil {
		return 0
	}
	msg, _ := strconv.ParseInt(string(b), 10, 64)
	return msg
}

// leftNotice says on track id's relay that the reviewer whose folder
// is dir left, and why, when one joined: its handle is kept as it joins. It
// follows what tmux did, so a caller hanging up doesn't stop it. The
// caller holds s.reviewMu.
func (s *Service) leftNotice(ctx context.Context, id, dir, why string) error {
	if _, err := os.Stat(filepath.Join(dir, "handle")); err != nil {
		return nil
	}
	return s.notice(context.WithoutCancel(ctx), id, "@"+reviewHandle(dir)+" left: "+why+".")
}

// notice posts body on track id's relay as Tracks' notice to everyone.
func (s *Service) notice(ctx context.Context, id, body string) error {
	_, err := s.Store.AddRelayMessage(ctx, id, relay.Message{From: relay.TracksHandle, Kind: relay.Notice, Body: body, At: s.now()})
	return err
}
