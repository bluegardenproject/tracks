package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/agents/claude"
	"github.com/bluegardenproject/tracks/internal/v2/agents/cursor"
	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/rpc"
	"github.com/bluegardenproject/tracks/internal/v2/settings"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/spf13/cobra"
)

// askTimeout bounds asking the daemon for the track, which only sets
// the base and the candor: without an answer the review still runs.
const askTimeout = 5 * time.Second

func newReviewCmd() *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "review",
		Short: "review this track's branch with a separate Cursor agent",
		Long: "Runs the tracks-v2-reviewer as a second Cursor agent with a fresh chat, " +
			"so it reads the diff without the working agent's history. Cursor can't " +
			"hand work to a custom subagent the way Claude Code does; this is the " +
			"equivalent for Cursor tracks.\n\n" +
			"The reviewer runs with no permission flags, so the approval policy " +
			"configured for Cursor applies unchanged.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			id := os.Getenv("TRACKS_ID")
			if id == "" {
				return errNoTrack("review")
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			t := askTrack(c.Context(), rpc.Client{Socket: paths.Socket}, id)
			if base == "" {
				base = reviewBase(c.Context(), dir, t)
			}
			diff, err := branchDiff(c.Context(), dir, base)
			if err != nil {
				return err
			}
			found, err := agents.Check(c.Context(), agents.Cursor)
			if err != nil {
				return err
			}
			var model string
			if s, err := settings.Load(paths.Settings); err == nil && s.Engines.Cursor != nil {
				model = s.Engines.Cursor.Model
			}

			fmt.Fprintln(c.ErrOrStderr(), "Running the reviewer in a separate agent session…")
			report, err := cursor.Review(c.Context(), cursor.ReviewRequest{
				Binary: found.Path, Workspace: dir, Diff: diff, Instructions: claude.ReviewerInstructions(),
				Candor: t.Candor, LockDir: filepath.Join(paths.DataDir, "locks"), TrackID: id, Model: model,
			})
			if errors.Is(err, cursor.ErrAlreadyReviewing) {
				return fmt.Errorf("%w: a reviewer doesn't review itself", err)
			}
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), report)
			return err
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "the ref to diff against (default origin/<base> of the track's repo)")
	return cmd
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
