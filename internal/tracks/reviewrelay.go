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
// handle of the review it replaces.
func newReviewHandle(old string) (string, error) {
	return train.NewHandle(train.Reviewer.Prefix(), func(h string) bool { return h == old })
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
	return writeWhole(filepath.Join(dir, string(kind)+".msg"), strconv.FormatInt(m.ID, 10))
}

// markPrinted marks read, for the track, the relay message of what
// tracks review printed: the report or an answer from dir, named by
// path. Marking is a reader's cursor, so what came before it on the
// relay counts as read too.
func (s *Service) markPrinted(ctx context.Context, id, path string) error {
	kind := relay.Report
	if strings.HasPrefix(filepath.Base(path), "answer") {
		kind = relay.Answer
	}
	b, err := os.ReadFile(filepath.Join(filepath.Dir(path), string(kind)+".msg"))
	if err != nil {
		return nil
	}
	msg, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return nil
	}
	return s.Store.MarkRelayRead(ctx, id, relay.TrackHandle, msg)
}

// notice posts body on track id's relay as Tracks' notice to everyone.
func (s *Service) notice(ctx context.Context, id, body string) error {
	_, err := s.Store.AddRelayMessage(ctx, id, relay.Message{From: relay.TracksHandle, Kind: relay.Notice, Body: body, At: s.now()})
	return err
}
