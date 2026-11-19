package cli

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/hooks"
	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/spf13/cobra"
)

// hookWait is how long a hook waits for the daemon.
const hookWait = time.Second

// newHookCmd is what the agents' hooks run. It never gets in the
// agent's way: it always succeeds, answers as if there were no hook,
// and logs what went wrong.
func newHookCmd() *cobra.Command {
	var engine, id, reviewer, replyFile string
	var answer bool
	cmd := &cobra.Command{
		Use:    "hook",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			paths, err := platform.Resolve()
			if err != nil {
				return nil
			}
			client := rpc.Client{Socket: paths.Socket}
			logf := func(format string, args ...any) {
				_ = appendLog(filepath.Join(paths.DataDir, "logs", "hook.log"), fmt.Sprintf(format, args...))
			}
			if reviewer != "" && replyFile != "" {
				sendReplyFile(c.Context(), replyFile, id, reviewer, answer, client.ReviewReply, logf)
				return nil
			}
			if reviewer != "" {
				runReviewerHook(c.Context(), os.Stdin, c.OutOrStdout(), engine, id, reviewer, client.ReviewReply, logf)
				return nil
			}
			runHook(c.Context(), os.Stdin, c.OutOrStdout(), engine, id, client.Report, logf)
			return nil
		},
	}
	cmd.Flags().StringVar(&engine, "engine", "", "the agent CLI: claude or cursor")
	cmd.Flags().StringVar(&id, "track", "", "the track's ID")
	cmd.Flags().StringVar(&reviewer, "reviewer", "", "the review run, for the hooks of a track's reviewer")
	cmd.Flags().StringVar(&replyFile, "reply-file", "", "with --reviewer: a file holding the reviewer's reply")
	cmd.Flags().BoolVar(&answer, "answer", false, "with --reply-file: the reply answers a question, not a review")
	return cmd
}

// runHook reads a hook's input from in, reports what it means for
// track id, and prints the engine's reply to out.
func runHook(ctx context.Context, in io.Reader, out io.Writer, engine, id string,
	report func(context.Context, rpc.ReportParams) error, logf func(string, ...any)) {
	p, err := hooks.Read(in)
	defer func() { fmt.Fprint(out, hooks.Reply(engine, p.Event)) }()
	if err != nil {
		logf("%s hook for %s: reading its input: %v", engine, id, err)
		return
	}
	e, _ := hooks.Event(engine, p)
	params := rpc.ReportParams{ID: id, Event: string(e), PRs: hooks.PRs(engine, p)}
	if params.Event == "" && len(params.PRs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, hookWait)
	defer cancel()
	if err := report(ctx, params); err != nil {
		logf("%s %s for %s: %v", engine, p.Event, id, err)
	}
}

// reviewReplyWait is how long a reviewer's hook waits for the daemon
// to take its reply, which may be a long report.
const reviewReplyWait = 5 * time.Second

// runReviewerHook reads a hook's input from in and hands the daemon the
// reviewer's reply it carries: a reviewer's reports are its replies, not
// the track's status.
func runReviewerHook(ctx context.Context, in io.Reader, out io.Writer, engine, id, run string,
	reply func(context.Context, rpc.ReviewReplyParams) error, logf func(string, ...any)) {
	p, err := hooks.Read(in)
	defer func() { fmt.Fprint(out, hooks.Reply(engine, p.Event)) }()
	if err != nil {
		logf("%s reviewer hook for %s: reading its input: %v", engine, id, err)
		return
	}
	text, ok := hooks.AgentReply(engine, p)
	if !ok || text == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, reviewReplyWait)
	defer cancel()
	if err := reply(ctx, rpc.ReviewReplyParams{ID: id, Run: run, Text: text}); err != nil {
		logf("%s reviewer %s for %s: %v", engine, p.Event, id, err)
	}
}

// sendReplyFile hands the daemon a reviewer's reply from a file, for an
// engine whose hooks don't carry it: Cursor's reviewer writes its
// review's turn to one.
func sendReplyFile(ctx context.Context, path, id, run string, answer bool,
	reply func(context.Context, rpc.ReviewReplyParams) error, logf func(string, ...any)) {
	b, err := os.ReadFile(path)
	if err != nil {
		logf("reviewer reply for %s: %v", id, err)
	}
	ctx, cancel := context.WithTimeout(ctx, reviewReplyWait)
	defer cancel()
	if err := reply(ctx, rpc.ReviewReplyParams{ID: id, Run: run, Text: replyFileText(string(b), answer)}); err != nil {
		logf("reviewer reply for %s: %v", id, err)
	}
}

// replyFileText is the report, or the answer, a reviewer's turn wrote.
// A turn that failed, such as on its sign-in or the network, wrote
// none: it says so, a report with an outcome line, so tracks review
// doesn't wait for one in vain.
func replyFileText(text string, answer bool) string {
	if answer {
		if strings.TrimSpace(text) == "" {
			return "The reviewer's turn ended without an answer: its pane, which the footer's button shows, says why."
		}
		return text
	}
	if strings.Contains(text, review.Outcome) {
		return text
	}
	if strings.TrimSpace(text) != "" {
		text = strings.TrimSpace(text) + "\n\n"
	}
	return text + "The reviewer's turn ended without a report: its pane, which the footer's button shows, says why.\n\n" +
		review.Outcome + " not run"
}

func appendLog(path, line string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	log.New(f, "", log.LstdFlags).Println(line)
	return nil
}
