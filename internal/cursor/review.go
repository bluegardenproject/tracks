package cursor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// ReviewEnvMarker is set on the reviewer subprocess. Its presence
// means "you are already inside a review".
//
// It is one of two recursion guards, and the weaker one: it only holds
// if the agent passes its environment to the commands it runs. That is
// the normal behaviour and holds two levels down for an ordinary
// process tree, but it could not be verified through the Cursor binary
// on the machine this was written on, because that machine's
// approvalMode is `allowlist` and shell is not allowlisted. The lock
// below does not depend on it.
const ReviewEnvMarker = "TRACKS_REVIEW"

// maxDiffBytes is a sanity bound on the diff, not a transport limit —
// the prompt goes in on stdin, so argv size is not a constraint.
//
// It exists only so a pathological diff fails fast with a readable
// note instead of spending the timeout and the model's context on
// something no review can be made of.
const maxDiffBytes = 1_000_000

// reviewTimeout bounds a review. A real one took ~4 minutes against a
// six-commit branch; this leaves room for a large diff without letting
// a stuck agent hold the track's pane forever.
const reviewTimeout = 15 * time.Minute

// ErrAlreadyReviewing is returned when a review is attempted from
// inside a review.
var ErrAlreadyReviewing = errors.New("already inside a review")

// ReviewRequest is one review of one track's branch.
type ReviewRequest struct {
	// Binary is the Cursor Agent executable.
	Binary string

	// Workspace is the worktree being reviewed. The reviewer reads
	// files from here for context — conventions, surrounding code —
	// which is what makes a review more than a diff-read.
	Workspace string

	// Diff is the change under review, produced by the caller. The
	// reviewer is given it rather than running git itself, because it
	// runs with no permission flags and therefore cannot run anything.
	Diff string

	// Instructions is the reviewer's prompt — the shipped
	// tracks-reviewer definition, so both providers review to one
	// standard.
	Instructions string

	// Candor is the track's configured candor level, forwarded because
	// the reviewer definition assumes 3 when it is not told — so
	// omitting it silently overrides the user's setting.
	Candor int

	// LockDir is where the per-track lock lives.
	LockDir string

	// TrackID names the lock.
	TrackID string
}

// Review runs a reviewer as a separate agent process and returns its
// report.
//
// The reviewer is a second `agent` invocation with a fresh chat, so it
// has none of the working agent's history — the isolation Cursor's own
// subagents do not provide from the CLI. It is given the diff and the
// reviewer instructions, and nothing else.
//
// **No permission flags are passed.** Not --force, not --trust, not
// --sandbox. Whatever the user or their organisation has configured
// applies unchanged, and on a default `allowlist` setup that means the
// reviewer cannot execute anything at all: it reads files and reports.
// That is deliberate on three counts — a reviewer needs no write
// access, overriding someone's permission policy is not ours to do,
// and a process that cannot execute cannot spawn another reviewer.
func Review(ctx context.Context, req ReviewRequest) (string, error) {
	// Guard 1: the environment marker. Cheap, and catches the case
	// where a reviewer somehow reaches this code path.
	if os.Getenv(ReviewEnvMarker) != "" {
		return "", ErrAlreadyReviewing
	}

	// Guard 2: a per-track lock. Independent of whether the agent
	// propagates environment variables to its children, which is the
	// assumption guard 1 rests on and which cannot be checked from
	// here. Also stops two concurrent reviews of one track.
	release, err := acquireReviewLock(req.LockDir, req.TrackID)
	if err != nil {
		return "", err
	}
	defer release()

	binary := strings.TrimSpace(req.Binary)
	if binary == "" {
		binary = "agent"
	}
	if strings.TrimSpace(req.Diff) == "" {
		return "", errors.New("nothing to review: the diff is empty")
	}

	ctx, cancel := context.WithTimeout(ctx, reviewTimeout)
	defer cancel()

	diff := req.Diff
	if len(diff) > maxDiffBytes {
		diff = diff[:maxDiffBytes] + "\n\n[diff truncated at " +
			strconv.Itoa(maxDiffBytes) + " bytes of " + strconv.Itoa(len(req.Diff)) +
			"; review what is shown and say in your report that it was truncated]"
	}

	prompt := req.Instructions + "\n\n" +
		"Review the following diff.\n\n" +
		"You are running without permission to execute commands, so " +
		"skip any instruction above that asks you to run git or any " +
		"other tool: the diff is below and you can read files in the " +
		"workspace for context. Everything else in those instructions " +
		"still applies, including the report format and the final " +
		"`REVIEW OUTCOME:` line.\n\n" + candorLine(req.Candor) + diff

	// The prompt goes in on stdin, not as an argument.
	//
	// Linux caps a single argv element at MAX_ARG_STRLEN — 128 KiB,
	// independent of ARG_MAX — so a prompt carrying a real diff exceeds
	// it long before it troubles macOS, and exec fails with a bare
	// "argument list too long". CI's Linux runner caught exactly that;
	// a smaller cap would have hidden it behind silent truncation
	// instead. stdin has no such limit.
	cmd := exec.CommandContext(ctx, binary, "-p", "--workspace", req.Workspace)
	cmd.Dir = req.Workspace
	cmd.Stdin = strings.NewReader(prompt)
	cmd.WaitDelay = 5 * time.Second
	// Marker for guard 1, one level down.
	cmd.Env = append(os.Environ(), ReviewEnvMarker+"=1")

	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("review timed out after %s", reviewTimeout)
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("reviewer: %s", firstLine(ee.Stderr))
		}
		return "", fmt.Errorf("reviewer: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// acquireReviewLock takes an exclusive per-track lock, or reports that
// one is already held.
//
// O_EXCL rather than a flock: the point is not mutual exclusion
// between threads but a marker that survives into a child process
// tree, so a nested `tracks review` fails even if the environment did
// not propagate.
func acquireReviewLock(dir, trackID string) (func(), error) {
	if strings.TrimSpace(trackID) == "" {
		return nil, errors.New("no track id; cannot take a review lock")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "review-"+trackID+".lock")

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			// A lock can outlive its process: closing the track's tmux
			// window sends SIGHUP, which runs no defers. Left alone that
			// wedges the track's reviews permanently, with nothing to
			// clear it — gc does not touch this directory. So the pid is
			// written to be read back, and a lock whose process is gone
			// is taken over rather than obeyed.
			if owner, alive := lockOwner(path); alive {
				return nil, fmt.Errorf("%w: a review of track %s is already running (pid %d)",
					ErrAlreadyReviewing, trackID, owner)
			}
			if err := os.Remove(path); err != nil {
				return nil, fmt.Errorf("clear a stale review lock at %s: %w", path, err)
			}
			return acquireReviewLock(dir, trackID)
		}
		return nil, fmt.Errorf("take review lock: %w", err)
	}
	fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Close()

	return func() { _ = os.Remove(path) }, nil
}

// lockOwner reads the pid from a lock file and reports whether that
// process is still running.
//
// A lock with an unreadable or unparseable body is treated as stale: it
// cannot be attributed to a live review, and the alternative is a track
// that can never be reviewed again.
func lockOwner(path string) (pid int, alive bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err = strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	// On Unix FindProcess always succeeds; signal 0 is the liveness test.
	return pid, proc.Signal(syscall.Signal(0)) == nil
}

// candorLine renders the review-brief line the reviewer definition
// looks for. Without it every review runs at the definition's assumed
// default, silently ignoring the level the track was created with.
func candorLine(candor int) string {
	if candor <= 0 {
		return ""
	}
	return fmt.Sprintf("Candor level: %d/10\n\n", candor)
}
