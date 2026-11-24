package tracks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/agents/cursor"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/store"
)

// reviewSession is how a track's reviewer runs, kept with its review
// so a question can reach it.
type reviewSession struct {
	Engine    string `json:"engine"`
	Program   string `json:"program"`
	Model     string `json:"model,omitempty"`
	Workspace string `json:"workspace"`
	// Chat is a Cursor reviewer's chat.
	Chat string `json:"chat,omitempty"`
	// Reply is the hook command that hands a reply to Tracks.
	Reply string `json:"reply"`
	// Tracks is the tracks command the reviewer's pane runs; "" in a
	// review started before it was kept.
	Tracks string `json:"tracks,omitempty"`
}

func (r reviewSession) save(dir string) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "session.json"), b, 0o600)
}

func loadSession(dir string) (reviewSession, error) {
	var r reviewSession
	b, err := os.ReadFile(filepath.Join(dir, "session.json"))
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(b, &r)
}

// cursorTurn is the script of a Cursor reviewer's turn on prompt: in
// print mode, written to out and handed to Tracks, then on in the same
// chat, interactively. answer marks a turn on a question.
func (r reviewSession) cursorTurn(prompt, out string, answer bool) string {
	reply := r.Reply
	name := agents.Cursor.Name
	if r.Model != "" {
		name += " (" + r.Model + ")"
	}
	stream, wait := "", " It shows nothing until it's done."
	if r.Tracks != "" {
		stream, wait = shellx.Quote(r.Tracks)+" review-stream", ""
	}
	status := name + " is reviewing the change." + wait + " When its report is done, you can ask it about it here."
	if answer {
		reply += " --answer"
		status = name + " is answering a question about its review." + wait
	}
	return cursor.Reviewer{
		Program: r.Program, Model: r.Model, Chat: r.Chat, Workspace: r.Workspace, Prompt: prompt,
		Report: out, Reply: reply + " --reply-file " + shellx.Quote(out), Status: status, Stream: stream,
	}.Script()
}

// AskReview asks track id's reviewer question, about its review, once
// it has reported and while its pane is hidden: a user who shows it
// talks to it there, and a reply of theirs would pass for the answer.
// WaitReview then waits for the answer. A Claude reviewer gets it typed
// into its pane, behind questionLead. A Cursor reviewer's hooks don't
// carry its replies when it runs interactively, so its pane is started
// again on a print-mode turn on the question, in the same chat, which
// ends what's typed in the pane but not sent.
func (s *Service) AskReview(ctx context.Context, id, question string) error {
	_, err := s.askReview(ctx, id, question)
	return err
}

// askReview is AskReview; it returns the question as posted on the
// track's relay.
func (s *Service) askReview(ctx context.Context, id, question string) (relay.Message, error) {
	if s.Reviewers == nil {
		return relay.Message{}, errors.New("reviews need tmux")
	}
	asked := strings.TrimSpace(question)
	if asked == "" {
		return relay.Message{}, Problem("Ask the reviewer something: tracks review --ask \"<question>\".")
	}
	if strings.ContainsFunc(asked, unicode.IsControl) {
		return relay.Message{}, Problem("Ask the reviewer on one line, without control characters.")
	}
	question = questionLead + asked
	t, err := s.Store.Track(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return relay.Message{}, Problem("That track is gone.")
	} else if err != nil {
		return relay.Message{}, err
	}
	s.reviewMu.Lock()
	defer s.reviewMu.Unlock()
	dir := s.reviewDir(id)
	window, _ := s.windowOf(id)
	r, err := s.Reviewers.Reviewer(id, window)
	if err != nil {
		return relay.Message{}, err
	}
	sess, sessErr := loadSession(dir)
	switch {
	case r.Pane == "" || sessErr != nil:
		return relay.Message{}, Problem(t.Name + " has no reviewer: tracks review starts one.")
	case r.Shown:
		return relay.Message{}, Problem("The reviewer's pane is open, so the user may be talking to it: ask there, or hide it " +
			"with the footer's button and ask again.")
	case !exists(filepath.Join(dir, "report.md")):
		return relay.Message{}, Problem("The reviewer hasn't reported yet: tracks review --wait waits for its report.")
	case s.asking(id):
		return relay.Message{}, Problem("The reviewer is still on the last question: tracks review --wait waits for its answer.")
	}
	// The question first, so a WaitReview meanwhile never takes the
	// report for the answer.
	if err := writeWhole(filepath.Join(dir, "question.md"), question); err != nil {
		return relay.Message{}, err
	}
	for _, old := range []string{"answer.md", "answer.out", "answer.msg"} {
		if err := os.Remove(filepath.Join(dir, old)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return relay.Message{}, err
		}
	}
	switch sess.Engine {
	case agents.Cursor.ID:
		script := sess.cursorTurn(question, filepath.Join(dir, "answer.out"), true)
		err = s.Reviewers.RespawnReviewer(r.Pane, sess.Workspace, s.reviewerCommand(id, script))
	default:
		err = s.Reviewers.TypeReviewer(r.Pane, question)
	}
	if err != nil {
		_ = os.Remove(filepath.Join(dir, "question.md"))
		return relay.Message{}, fmt.Errorf("ask the reviewer: %w", err)
	}
	// Posted whatever the caller does now: it's typed in already.
	return s.Store.AddRelayMessage(context.WithoutCancel(ctx), id, relay.Message{From: relay.TrackHandle, To: reviewHandle(dir),
		Kind: relay.Question, Body: asked, At: s.now()})
}

// questionLead starts every question. Typed into Claude's input box, a
// question's first character could switch it to a shell command (!), a
// slash command (/) or memory (#), which would run in the reviewer's
// pane outside any sandbox; behind the lead it's only ever a message.
const questionLead = "A question about your review: "

// asking reports whether track id's reviewer was asked a question it
// hasn't answered. The caller holds s.reviewMu.
func (s *Service) asking(id string) bool {
	dir := s.reviewDir(id)
	return exists(filepath.Join(dir, "question.md")) && !exists(filepath.Join(dir, "answer.md"))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
