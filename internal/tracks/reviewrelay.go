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

// reviewHandle is the handle of the review in dir: its own, given as it
// started, or the reviewer's prefix alone for a review from before.
func reviewHandle(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "handle"))
	if err != nil || train.CheckHandle(string(b)) != nil {
		return train.ReviewerHandle
	}
	return string(b)
}

// newReviewHandle is a handle for a new review that isn't old's, the
// handle of the review it replaces. prefix is the setting's, the
// reviewer's name when it's unset or isn't one.
func newReviewHandle(prefix, old string) (string, error) {
	if train.CheckPrefix(prefix) != nil {
		prefix = train.Reviewer.Prefix()
	}
	return train.NewHandle(prefix, func(h string) bool { return h == old })
}

// postReview posts text, the report or an answer of track id's reviewer,
// as its message to the track, and keeps the message's number in dir, so
// the tracks review that prints it can mark it read. The caller holds
// s.reviewMu.
func (s *Service) postReview(ctx context.Context, id, dir string, kind relay.Kind, text string) error {
	m, err := s.Store.AddRelayMessage(ctx, id, relay.Message{From: reviewHandle(dir), To: relay.TrackHandle,
		Kind: kind, Body: text, At: s.now()})
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

// leftNotice says on track id's relay that the reviewer of the review in
// dir left, and why, when one joined: its handle is kept as it joins. It
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
