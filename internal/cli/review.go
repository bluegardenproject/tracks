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

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/agents/claude"
	"github.com/bluegardenproject/tracks/internal/agents/cursor"
	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/spf13/cobra"
)

// askTimeout bounds asking the daemon for the track, which only sets
// the base and the candor: without an answer the review still runs.
const askTimeout = 5 * time.Second

func newReviewCmd() *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "review",
		Short: "review this track's branch with a separate agent session",
		Long: "Runs the tracks-reviewer in a fresh agent session, so it reads the diff without the working " +
			"agent's history. It runs on the engine and model of the reviewer setting in settings.yaml, by " +
			"default the track's own engine. The reviewer only reads: Cursor's runs without permission " +
			"flags, Claude's with read-only tools, and the worktree is checked for changes afterwards.",
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
			set, err := settings.Load(paths.Settings)
			if err != nil {
				return fmt.Errorf("the reviewer setting can't be read: %w", err)
			}
			engineID, model := set.ReviewerOn(trackEngine(t.Engine))
			command, err := reviewCommand(c.Context(), engineID, model, dir)
			if err != nil {
				return err
			}

			before, err := worktreeState(c.Context(), dir)
			if err != nil {
				return err
			}
			fmt.Fprintf(c.ErrOrStderr(), "Running the reviewer on %s in a separate agent session…\n", reviewerName(engineID, model))
			report, err := review.Run(c.Context(), review.Request{
				Command: command, Workspace: dir, Diff: diff, Instructions: claude.ReviewerInstructions(),
				Candor: t.Candor, LockDir: filepath.Join(paths.DataDir, "locks"), TrackID: id,
			})
			if errors.Is(err, review.ErrAlreadyReviewing) {
				return fmt.Errorf("%w: a reviewer doesn't review itself", err)
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
	return cmd
}

// reviewCommand is how engine id starts a reviewer of dir on model.
func reviewCommand(ctx context.Context, id, model, dir string) (review.Command, error) {
	e, ok := agents.ByID(id)
	if !ok {
		return review.Command{}, fmt.Errorf("tracks doesn't know the reviewer's engine %q", id)
	}
	var command func(binary string) review.Command
	switch id {
	case agents.Claude.ID:
		command = func(binary string) review.Command { return claude.ReviewCommand(binary, model) }
	case agents.Cursor.ID:
		command = func(binary string) review.Command { return cursor.ReviewCommand(binary, dir, model) }
	default:
		return review.Command{}, fmt.Errorf("%s can't review yet", e.Name)
	}
	found, err := agents.Check(ctx, e)
	if err != nil {
		return review.Command{}, err
	}
	return command(found.Path), nil
}

// reviewerName names the engine and model a review runs on.
func reviewerName(id, model string) string {
	if e, ok := agents.ByID(id); ok {
		id = e.Name
	}
	if model == "" {
		return id
	}
	return id + " (" + model + ")"
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

// trackEngine is the engine of the track under review: as the daemon
// has it, else the agent running this command, told by its
// environment. Claude Code sets CLAUDECODE in its shells; Cursor, the
// other engine, sets nothing to tell it by.
func trackEngine(known string) string {
	switch {
	case known != "":
		return known
	case os.Getenv("CLAUDECODE") != "":
		return agents.Claude.ID
	}
	return agents.Cursor.ID
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
