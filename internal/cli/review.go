package cli

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/bluegardenproject/tracks/internal/train"
	"github.com/spf13/cobra"
)

// askTimeout bounds asking the daemon for the track, which only sets
// the base: without an answer the base is guessed.
const askTimeout = 5 * time.Second

func newReviewCmd() *cobra.Command {
	var base, ask, brief, owner, to string
	var wait, stale, all bool
	cmd := &cobra.Command{
		Use:   "review",
		Short: "review this track's branch with the track's reviewer",
		Long: "Starts the track's reviewer on the branch's diff and waits for its report. The reviewer is a " +
			"session of its own, on the agent and model picked under Settings → Trains (agents) → Reviewer, in a tmux " +
			"session of its own that the user opens in an overlay (tracks train show <handle>). It only reads. A review " +
			"ends the reviewer of the same checkout that's there and starts a fresh one; reviewers of other checkouts " +
			"run on beside it. It prints the reviewer's handle as it starts. --ask asks the reviewer that's there about " +
			"its review and prints its answer; --wait waits for the report of the one that's running, or its " +
			"answer to the last question; with more than one reviewer, --to names the one meant. A review covers the whole change. --owner alone narrows it to the " +
			"files the repo's Code owner owns by CODEOWNERS, as the Repositories tab sets it; --owner <handle> to " +
			"another's.",
		Args: func(c *cobra.Command, args []string) error {
			// --owner takes no value when it's alone, so a handle after it
			// comes as an argument.
			if len(args) == 1 && owner == repoOwner {
				return c.Flags().Set("owner", args[0])
			}
			return cobra.NoArgs(c, args)
		},
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
			case c.Flags().Changed("to") && !wait && !asking:
				return errors.New("--to names the reviewer for --wait or --ask; a new review starts a reviewer of its own")
			case c.Flags().Changed("owner") && strings.TrimSpace(owner) == "":
				return errors.New("--owner needs a code owner, such as @org/team")
			case all && c.Flags().Changed("owner"):
				return errors.New("--all and --owner can't go together")
			case (wait || asking) && (c.Flags().Changed("base") || c.Flags().Changed("brief") || c.Flags().Changed("owner") || stale || all):
				return errors.New("--base, --brief, --owner, --all and --stale are for a new review, not with --wait or --ask")
			}
			var packet review.Packet
			if !wait && !asking {
				tr := askTrack(c.Context(), client, id)
				if base == "" {
					base = reviewBase(c.Context(), dir, tr)
				}
				if packet, err = gatherPacket(c.Context(), dir, base); err != nil {
					return err
				}
				fromRepo := owner == repoOwner
				if fromRepo {
					if owner, err = codeOwner(c.Context(), client, id, dir); err != nil {
						return err
					}
					progress("Reviewing only the files " + owner + " owns, the repo's Code owner.")
				}
				if strings.TrimSpace(owner) != "" {
					if packet, err = ownedPacket(c.Context(), dir, packet, owner); err != nil {
						if fromRepo {
							return fmt.Errorf("the repo's Code owner is %s, but %w; without --owner the whole change is reviewed", owner, err)
						}
						return err
					}
				}
				if brief != "" {
					if packet.Brief, err = readBrief(c.InOrStdin(), brief); err != nil {
						return err
					}
				}
			}
			// The worktree is checked here, in the agent's sandbox: the
			// daemon runs no git, since a repo's config can make git run
			// commands. --wait checks the time it waits.
			if wait || asking {
				// What's checked is the checkout of the reviewer
				// addressed, which needn't be where this runs.
				dir = cmp.Or(addressedTarget(c.Context(), client, id, to), dir)
			}
			before, err := worktreeState(c.Context(), dir)
			if err != nil {
				return err
			}
			var report string
			switch {
			case wait:
				report, err = client.ReviewWait(c.Context(), id, to, progress)
			case asking:
				report, err = client.ReviewAsk(c.Context(), id, to, ask, progress)
			default:
				report, err = client.Review(c.Context(), tracks.ReviewRequest{ID: id, Dir: dir, Packet: packet.Fit(), Stale: stale}, progress)
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
	cmd.Flags().StringVar(&brief, "brief", "", "a file, or - for stdin, with what the user asks of this review")
	cmd.Flags().StringVar(&owner, "owner", "", "review only the files a code owner owns, by the repo's CODEOWNERS: alone, "+
		"the repo's Code owner; or a handle, such as @org/team")
	cmd.Flags().Lookup("owner").NoOptDefVal = repoOwner
	cmd.Flags().BoolVar(&all, "all", false, "review the whole change, as without --owner")
	cmd.Flags().BoolVar(&stale, "stale", false, "review a pull request's worktree that's behind the pull request")
	cmd.Flags().StringVar(&to, "to", "", "with --wait or --ask: the reviewer meant, by handle or a prefix only it has, "+
		"when more than one runs")
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

// addressedTarget is the checkout of track id's reviewer that --to
// addresses, its only one for "", as the daemon has it; "" when it
// can't say.
func addressedTarget(ctx context.Context, client rpc.Client, id, to string) string {
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()
	trains, err := client.TrainList(ctx, id)
	if err != nil {
		return ""
	}
	return reviewerTarget(trains, to)
}

// reviewerTarget is the target of the reviewer among trains that to
// names, as train.Resolve resolves it, the only one for ""; "" for none.
func reviewerTarget(trains []rpc.Train, to string) string {
	var reviewers []rpc.Train
	var handles []string
	for _, t := range trains {
		if t.Agent == train.Reviewer.Name() && t.Target != "" {
			reviewers, handles = append(reviewers, t), append(handles, t.Handle)
		}
	}
	if to == "" {
		if len(reviewers) == 1 {
			return reviewers[0].Target
		}
		return ""
	}
	h, err := train.Resolve(to, handles)
	if err != nil {
		return ""
	}
	return reviewers[slices.Index(handles, h)].Target
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

// gatherPacket is the change under review and what's around it: the
// branch's own commits and what isn't committed yet, since the review
// runs before a push, which is often before the last commit.
func gatherPacket(ctx context.Context, dir, base string) (review.Packet, error) {
	if !validBaseRef(base) {
		return review.Packet{}, fmt.Errorf("invalid base ref %q", base)
	}
	p := review.Packet{Base: base}
	for _, part := range []struct {
		rng         []string
		uncommitted bool
	}{{[]string{"--end-of-options", base + "...HEAD", "--"}, false}, {[]string{"--end-of-options", "HEAD", "--"}, true}} {
		// The same paths in both, whatever the user's diff config.
		diff, err := git(ctx, dir, append(diffArgs, part.rng...)...)
		if err != nil {
			return p, fmt.Errorf("git diff %s: %w", part.rng[1], err)
		}
		numstat, err := git(ctx, dir, append(append(diffArgs, "--numstat", "-z"), part.rng...)...)
		if err != nil {
			return p, fmt.Errorf("git diff --numstat %s: %w", part.rng[1], err)
		}
		p.Diffs = append(p.Diffs, review.SplitDiff(diff, part.uncommitted)...)
		p.Files = review.MergeFiles(p.Files, review.ParseNumstat(numstat))
	}
	if p.Empty() {
		return p, fmt.Errorf("no changes against %s: nothing to review", base)
	}
	commits, err := git(ctx, dir, "log", "--no-merges", "--no-color", "--format=### %h %s%n%n%b", "--end-of-options", base+"..HEAD", "--")
	if err != nil {
		return p, fmt.Errorf("git log %s..HEAD: %w", base, err)
	}
	p.Commits = strings.TrimSpace(commits)
	head, err := git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return p, fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	p.Head = strings.TrimSpace(head)
	return p, nil
}

// validBaseRef rejects option-like or whitespace/control base refs so
// --base cannot inject git flags.
func validBaseRef(base string) bool {
	if base == "" || strings.HasPrefix(base, "-") || strings.Contains(base, ":") {
		return false
	}
	for _, r := range base {
		if r <= 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// diffArgs run git diff with plain output: paths unquoted where they
// can be, a/ and b/ prefixes, no renames, colours or external tools.
var diffArgs = []string{"-c", "core.quotePath=false", "diff", "--no-ext-diff", "--no-textconv", "--no-color",
	"--no-renames", "--src-prefix=a/", "--dst-prefix=b/"}

// repoOwner is --owner without a handle: the repo's Code owner. It's no
// handle anyone has.
const repoOwner = "@"

// codeOwner is the Code owner of dir's repo, which --owner alone
// narrows a review to; an error when the repo has none.
func codeOwner(ctx context.Context, client rpc.Client, id, dir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, askTimeout)
	defer cancel()
	owner, err := client.ReviewOwner(ctx, id, dir)
	if err != nil {
		return "", fmt.Errorf("ask the daemon for the repo's Code owner: %w", err)
	}
	if owner == "" {
		return "", errors.New("the repo has no Code owner on the Repositories tab: set one there, or name one with --owner <handle>")
	}
	return owner, nil
}

// ownedPacket narrows p to the files owner owns by the CODEOWNERS on
// p's base, as GitHub reads it, so a branch can't change its own
// scope. With no CODEOWNERS, or nothing owner owns, there's nothing to
// narrow to, and the review stops rather than review all.
func ownedPacket(ctx context.Context, dir string, p review.Packet, owner string) (review.Packet, error) {
	if !validBaseRef(p.Base) {
		return p, fmt.Errorf("invalid base ref %q", p.Base)
	}
	var file string
	found := false
	for _, path := range review.CodeownersPaths {
		if text, err := git(ctx, dir, "show", "--end-of-options", p.Base+":"+path); err == nil {
			file, found = text, true
			break
		}
	}
	if !found {
		return p, fmt.Errorf("there's no CODEOWNERS on %s (looked in %s), so --owner can't tell what %s owns",
			p.Base, strings.Join(review.CodeownersPaths, ", "), review.NormalOwner(owner))
	}
	p = p.OwnedBy(review.ParseCodeowners(file), owner)
	if p.Scope.Owned == 0 {
		return p, fmt.Errorf("%s owns none of the %d changed files, by CODEOWNERS: nothing to review",
			p.Scope.Owner, p.Scope.Total)
	}
	return p, nil
}

// readBrief reads --brief's file, - for in.
func readBrief(in io.Reader, path string) (string, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(in)
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("read the brief: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
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
