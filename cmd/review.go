package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/cursor"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/spf13/cobra"
)

func init() {
	c := &cobra.Command{
		Use:   "review",
		Short: "review this track's branch with a separate agent",
		Long: "Runs the tracks code reviewer as a second agent process with a fresh " +
			"conversation, so it reads the diff without the working agent's history.\n\n" +
			"Exists because the Cursor CLI cannot delegate to a custom subagent: its " +
			"Task tool accepts only built-in types, and invoking a custom agent by " +
			"slash loads the instructions into the *same* conversation, which is not " +
			"a review by fresh eyes. Claude Code has real subagents and uses them " +
			"directly; this is the equivalent for Cursor tracks.\n\n" +
			"The reviewer is spawned with no permission flags, so whatever approval " +
			"policy is configured applies unchanged — on a default allowlist setup it " +
			"can read files and nothing else.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			trackID := os.Getenv("TRACKS_ID")
			if trackID == "" {
				return errors.New("$TRACKS_ID not set — `tracks review` only works from inside a track")
			}
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			base, _ := c.Flags().GetString("base")
			if base == "" {
				base = trackBase(cfg, trackID)
			}
			diff, err := branchDiff(base)
			if err != nil {
				return err
			}
			instructions, err := reviewerInstructions()
			if err != nil {
				return err
			}

			stateDir, err := cfg.ResolveStateDir()
			if err != nil {
				return fmt.Errorf("resolve state dir: %w", err)
			}

			fmt.Fprintln(os.Stderr, "running the reviewer in a separate agent session...")
			report, err := cursor.Review(c.Context(), cursor.ReviewRequest{
				Binary:       cfg.Cursor.Binary,
				Candor:       trackCandor(cfg, trackID),
				Workspace:    mustCwd(),
				Diff:         diff,
				Instructions: instructions,
				LockDir:      filepath.Join(stateDir, "locks"),
				TrackID:      trackID,
			})
			if errors.Is(err, cursor.ErrAlreadyReviewing) {
				// The recursion guard. Worth an explicit message: a
				// reviewer that tried to review would otherwise look
				// like a mysterious failure.
				return fmt.Errorf("%w — a reviewer does not review itself", err)
			}
			if err != nil {
				return err
			}
			fmt.Println(report)
			return nil
		},
	}
	c.Flags().String("base", "", "base ref to diff against (default: the track repo's configured base)")
	register(c)
}

// branchDiff produces the change under review. The caller runs git so
// the reviewer does not have to — it is spawned without permission to
// run anything.
func branchDiff(base string) (string, error) {
	if base == "" {
		base = fallbackBase()
	}
	// base...HEAD is the branch's own work; a plain `git diff HEAD`
	// catches anything not yet committed. The gate fires before a push,
	// which is often before the last commit, so both are included —
	// otherwise a reviewer reads stale content or nothing at all.
	out, err := exec.Command("git", "diff", base+"...HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git diff %s...HEAD: %s", base, gitErr(err))
	}
	uncommitted, err := exec.Command("git", "diff", "HEAD").Output()
	if err != nil {
		return "", fmt.Errorf("git diff HEAD: %s", gitErr(err))
	}
	if strings.TrimSpace(string(uncommitted)) != "" {
		out = append(out, []byte("\n\n--- uncommitted changes (not yet in a commit) ---\n")...)
		out = append(out, uncommitted...)
	}
	if strings.TrimSpace(string(out)) == "" {
		return "", fmt.Errorf("no changes against %s — nothing to review", base)
	}
	return string(out), nil
}

// trackBase resolves the ref a track's work should be diffed against.
//
// The repo's configured Base is authoritative: it is what the worktree
// was branched from (handlers.go creates from origin/<base>), and it is
// what the dashboard already uses. Guessing instead is actively wrong
// on a repo based on anything but main — origin/main usually exists
// there too, so a guess silently succeeds and hands the reviewer every
// commit the real base has that main doesn't, on top of the track's own.
func trackBase(cfg config.Config, trackID string) string {
	if t, ok, err := daemon.NewClient(cfg).Get(trackID); err == nil && ok {
		for _, tr := range t.Repos {
			if repo, ok := cfg.RepoByName(tr.Name); ok && strings.TrimSpace(repo.Base) != "" {
				return "origin/" + repo.Base
			}
		}
	}
	// Only reached when the daemon is unreachable or the track has no
	// configured repo — a worktree-less kind, say.
	return fallbackBase()
}

// fallbackBase guesses, and is only used when there is no configured
// base to read.
func fallbackBase() string {
	for _, candidate := range []string{"origin/main", "origin/master", "origin/develop"} {
		if err := exec.Command("git", "rev-parse", "--verify", "--quiet", candidate).Run(); err == nil {
			return candidate
		}
	}
	return "HEAD~1"
}

// frontmatter matches the YAML block the agent definitions open with.
var frontmatter = regexp.MustCompile(`(?s)\A---\n.*?\n---\n`)

// reviewerInstructions reads the shipped tracks-reviewer definition and
// strips its frontmatter, leaving the prompt body.
//
// Deliberately the same file Claude's subagent uses, so a Cursor track
// and a Claude track are reviewed to one standard rather than two that
// drift.
func reviewerInstructions() (string, error) {
	// The compiled-in template is the source of truth. Reading the
	// installed file instead would fail outright where the daemon has
	// never run, and — since tracks will not overwrite a file it does
	// not own — would silently use a user's own tracks-reviewer.md as
	// the reviewer prompt. That is precisely the drift this is meant to
	// avoid.
	body := strings.TrimSpace(frontmatter.ReplaceAllString(daemon.ReviewerAgentTemplate, ""))
	if body == "" {
		return "", errors.New("the built-in reviewer definition is empty")
	}
	return body, nil
}

// trackCandor reads the track's configured candor level, so a Cursor
// review is delivered at the bluntness the user chose rather than the
// reviewer definition's assumed default.
func trackCandor(cfg config.Config, trackID string) int {
	if t, ok, err := daemon.NewClient(cfg).Get(trackID); err == nil && ok {
		return t.CandorLevel()
	}
	return 0
}

// gitErr surfaces git's stderr, which .Output() hides inside ExitError.
func gitErr(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) && len(ee.Stderr) > 0 {
		return strings.TrimSpace(string(ee.Stderr))
	}
	return err.Error()
}

func mustCwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
