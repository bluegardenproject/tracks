package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/spf13/cobra"
)

// askTimeout bounds asking the daemon for the track, which only sets
// the base: without an answer the base is guessed.
const askTimeout = 5 * time.Second

func newReviewCmd() *cobra.Command {
	var base, ask string
	var wait bool
	cmd := &cobra.Command{
		Use:   "review",
		Short: "review this track's branch with the track's reviewer",
		Long: "Starts the track's reviewer on the branch's diff and waits for its report. The reviewer is a " +
			"session of its own, on the agent and model picked under Settings → Agents → Reviewer, in a pane of " +
			"the track that stays hidden until it's shown with the footer's button. It only reads. A review " +
			"ends the reviewer that's there and starts a fresh one. --ask asks the reviewer that's there about " +
			"its review and prints its answer; --wait waits for the report of the one that's running, or its " +
			"answer to the last question.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if os.Getenv(review.EnvMarker) != "" {
				return fmt.Errorf("%w: a reviewer doesn't start reviewers", review.ErrAlreadyReviewing)
			}
			id := os.Getenv("TRACKS_ID")
			if id == "" {
				return errNoTrack("review")
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			client := rpc.Client{Socket: paths.Socket}
			progress := func(step string) { fmt.Fprintln(c.ErrOrStderr(), step) }
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			asking := c.Flags().Changed("ask")
			switch {
			case wait && asking:
				return errors.New("--wait and --ask can't go together")
			case (wait || asking) && c.Flags().Changed("base"):
				return errors.New("--base is for a new review, not with --wait or --ask")
			}
			var diff string
			if !wait && !asking {
				if base == "" {
					base = reviewBase(c.Context(), dir, askTrack(c.Context(), client, id))
				}
				if diff, err = branchDiff(c.Context(), dir, base); err != nil {
					return err
				}
			}
			// The worktree is checked here, in the agent's sandbox: the
			// daemon runs no git, since a repo's config can make git run
			// commands. --wait checks the time it waits.
			before, err := worktreeState(c.Context(), dir)
			if err != nil {
				return err
			}
			var report string
			switch {
			case wait:
				report, err = client.ReviewWait(c.Context(), id, progress)
			case asking:
				report, err = client.ReviewAsk(c.Context(), id, ask, progress)
			default:
				report, err = client.Review(c.Context(), tracks.ReviewRequest{ID: id, Dir: dir, Diff: review.Truncate(diff)}, progress)
			}
			if err != nil {
				return err
			}
			after, stateErr := worktreeState(c.Context(), dir)
			changed := stateErr == nil && after != before
			if changed {
				report = "⛔ The worktree changed while the reviewer ran. A reviewer never changes anything: " +
					"check what changed before you go on.\n\n" + report
			}
			if _, err := fmt.Fprintln(c.OutOrStdout(), report); err != nil {
				return err
			}
			switch {
			case stateErr != nil:
				return fmt.Errorf("couldn't check the worktree is as it was before the review: %w", stateErr)
			case changed:
				return errors.New("the worktree changed while the reviewer ran")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "the ref to diff against (default origin/<base> of the track's repo)")
	cmd.Flags().BoolVar(&wait, "wait", false, "wait for the report of the review that's running, or its answer to the last question")
	cmd.Flags().StringVar(&ask, "ask", "", "ask the reviewer that's there about its review, and print its answer")
	return cmd
}

// worktreeState is what a reviewer must leave as it is: HEAD, git
// status, the changes to tracked files and the untracked files'
// contents, since a file that was dirty before stays the same line in
// git status after another edit.
func worktreeState(ctx context.Context, dir string) (string, error) {
	top, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	dir = strings.TrimSpace(top)
	var state strings.Builder
	for _, args := range [][]string{
		{"rev-parse", "HEAD"},
		{"status", "--porcelain"},
		{"diff", "HEAD", "--binary"},
	} {
		out, err := git(ctx, dir, args...)
		if err != nil {
			return "", err
		}
		state.WriteString(out)
	}
	untracked, err := git(ctx, dir, "ls-files", "-z", "--others", "--exclude-standard")
	if err != nil {
		return "", err
	}
	for _, name := range strings.FieldsFunc(untracked, func(r rune) bool { return r == 0 }) {
		state.WriteString(name + " " + fileState(filepath.Join(dir, name)) + "\n")
	}
	return state.String(), nil
}

// fileState stands for an untracked file's contents: a link's target,
// a file's hash. git hash-object refuses links, and an untracked
// symlink, such as a linked node_modules, must not stop a review.
func fileState(path string) string {
	info, err := os.Lstat(path)
	switch {
	case err != nil:
		return "gone"
	case info.Mode()&os.ModeSymlink != 0:
		target, _ := os.Readlink(path)
		return "link " + target
	case !info.Mode().IsRegular():
		return info.Mode().String()
	}
	f, err := os.Open(path)
	if err != nil {
		return "unreadable"
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "unreadable"
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// askTrack is the track with id as the daemon has it, or an empty one
// when the daemon can't say.
func askTrack(ctx context.Context, client rpc.Client, id string) track.Track {
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()
	listed, err := client.List(ctx)
	if err != nil {
		return track.Track{}
	}
	for _, l := range listed {
		if l.ID == id {
			return l.Track
		}
	}
	return track.Track{}
}

// reviewBase is origin/<base> of the track repo dir is in: what its
// worktree was branched from. Guessing instead is wrong on a repo based
// on anything but main, since origin/main usually exists there too.
func reviewBase(ctx context.Context, dir string, t track.Track) string {
	if top, err := git(ctx, dir, "rev-parse", "--show-toplevel"); err == nil {
		for _, r := range t.Repos {
			if r.Worktree != "" && r.Base != "" && samePath(r.Worktree, strings.TrimSpace(top)) {
				return "origin/" + r.Base
			}
		}
	}
	return fallbackBase(ctx, dir)
}

// fallbackBase guesses, for a repo the track doesn't list.
func fallbackBase(ctx context.Context, dir string) string {
	for _, candidate := range []string{"origin/main", "origin/master", "origin/develop"} {
		if _, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", candidate); err == nil {
			return candidate
		}
	}
	return "HEAD~1"
}

// branchDiff is the change under review: the branch's own commits and
// what isn't committed yet. The review runs before a push, which is
// often before the last commit.
func branchDiff(ctx context.Context, dir, base string) (string, error) {
	out, err := git(ctx, dir, "diff", base+"...HEAD")
	if err != nil {
		return "", fmt.Errorf("git diff %s...HEAD: %w", base, err)
	}
	uncommitted, err := git(ctx, dir, "diff", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git diff HEAD: %w", err)
	}
	if strings.TrimSpace(uncommitted) != "" {
		out += "\n\n--- uncommitted changes (not yet in a commit) ---\n" + uncommitted
	}
	if strings.TrimSpace(out) == "" {
		return "", fmt.Errorf("no changes against %s: nothing to review", base)
	}
	return out, nil
}

// git runs git in dir and returns its output, or its stderr as the
// error.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && len(exit.Stderr) > 0 {
		return "", errors.New(strings.TrimSpace(string(exit.Stderr)))
	}
	return string(out), err
}
