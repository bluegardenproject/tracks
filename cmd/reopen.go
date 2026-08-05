package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tui/menu"
	"github.com/spf13/cobra"
)

func init() {
	c := &cobra.Command{
		Use:   "reopen [track-id...]",
		Short: "bring back the tracks that were running when tracks was last quit",
		Long: "Resumes every track left in the `interrupted` status — the ones that were still live when " +
			"tracks was last shut down. Each gets its worktree back (if it was removed) and a fresh tmux " +
			"window running `claude --resume`, so the conversation continues where it stopped. Pass track " +
			"IDs to reopen only those. Use `tracks resume <id>` for a track that finished normally.",
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			cl := daemon.NewClient(cfg)

			// With no IDs, say up front whether there's anything to do —
			// otherwise an empty result reads like a failure.
			if len(args) == 0 {
				pending, err := interruptedTracks(cl)
				if err != nil {
					return err
				}
				if len(pending) == 0 {
					fmt.Println("nothing to reopen — no interrupted tracks")
					return nil
				}
				fmt.Printf("reopening %d interrupted track(s)...\n\n", len(pending))
			} else {
				fmt.Printf("reopening %d track(s)...\n\n", len(args))
			}

			res, err := cl.ReopenWithProgress(args, func(msg string) {
				fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
			})
			if err != nil {
				return fmt.Errorf("daemon: %w", err)
			}
			fmt.Println()
			printReopenResult(res)

			// Land the user on the reopened window when there's exactly one;
			// with several, leave the current window alone (the dashboard
			// lists them all).
			if len(res.Reopened) == 1 {
				tm := tmux.New()
				if tm.HasSession(cfg.Tmux.SessionName) && res.Reopened[0].WindowName != "" {
					_ = tm.SelectWindow(cfg.Tmux.SessionName, res.Reopened[0].WindowName)
				}
			}
			if len(res.Failed) > 0 {
				return fmt.Errorf("%d track(s) could not be reopened", len(res.Failed))
			}
			return nil
		},
	}
	register(c)
}

// offerReopen asks whether to bring back the tracks that were live when
// tracks last stopped, and does it when the user says yes. Called from
// bootstrap after the daemon is up (so state is reconciled) and before
// tmux takes the terminal.
//
// Every failure path is silent-and-continue: this is a convenience on
// the way into the session, and nothing here is worth blocking the
// user's `tracks` invocation over. The tracks stay interrupted and
// `tracks reopen` is still there.
func offerReopen(cfg config.Config) {
	if !stdinIsTTY() {
		return
	}
	cl := daemon.NewClient(cfg)
	pending, err := interruptedTracks(cl)
	if err != nil || len(pending) == 0 {
		return
	}

	fmt.Printf("\n%d track(s) were still running when tracks last stopped:\n", len(pending))
	for _, t := range pending {
		fmt.Printf("  %s  %s\n", lastN(t.ID, 15), trackLabel(t))
	}
	fmt.Println()

	yes, err := menu.Confirm(
		fmt.Sprintf("Reopen %d interrupted track(s)?", len(pending)),
		"Each gets its worktree back and a fresh window running claude --resume, "+
			"continuing the conversation where it stopped. Cancel leaves them as they are.")
	if err != nil || !yes {
		fmt.Println("left as they are — run `tracks reopen` when you want them back.")
		return
	}

	fmt.Println()
	res, err := cl.ReopenWithProgress(nil, func(msg string) {
		fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "reopen failed: %v\n", err)
		return
	}
	fmt.Println()
	printReopenResult(res)
}

// trackLabel is the short human name for a track in the reopen listing:
// slug, else branch, else the first words of the task prompt. The last
// fallback matters for worktree-less kinds (doc / ask / plan), which have
// no branch — without it an unslugged ask track would list as a bare ID.
func trackLabel(t state.Track) string {
	if t.Slug != "" {
		return t.Slug
	}
	if t.Branch != "" {
		return t.Branch
	}
	prompt := strings.TrimSpace(strings.SplitN(t.TaskPrompt, "\n", 2)[0])
	if prompt == "" {
		return "—"
	}
	const max = 40
	if len(prompt) > max {
		return prompt[:max-1] + "…"
	}
	return prompt
}

// stdinIsTTY reports whether stdin is a terminal — the precondition for
// putting an interactive prompt in front of the user. False when tracks
// is run from a script or with input piped in, where a blocking prompt
// would hang.
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// interruptedTracks returns every track waiting to be reopened, oldest
// first — the same set `tracks reopen` acts on with no arguments.
func interruptedTracks(cl *daemon.Client) ([]state.Track, error) {
	tracks, err := cl.Ls()
	if err != nil {
		return nil, fmt.Errorf("daemon: %w", err)
	}
	var out []state.Track
	for _, t := range tracks {
		if t.Status == state.StatusInterrupted {
			out = append(out, t)
		}
	}
	return out, nil
}

// printReopenResult writes the per-track outcome. Failures go to stderr
// so a caller piping stdout still sees them.
func printReopenResult(res daemon.ReopenResult) {
	for _, r := range res.Reopened {
		fmt.Printf("reopened %s → window %s\n", lastN(r.ID, 15), r.WindowName)
	}
	for _, f := range res.Failed {
		fmt.Fprintf(os.Stderr, "could not reopen %s: %s\n", lastN(f.ID, 15), f.Error)
	}
	if len(res.Reopened) == 0 && len(res.Failed) == 0 {
		fmt.Println("nothing to reopen — no interrupted tracks")
	}
}
