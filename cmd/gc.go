package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/git"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/spf13/cobra"
)

var gcPruneBranches bool

func init() {
	c := &cobra.Command{
		Use:   "gc",
		Short: "garbage-collect orphaned worktrees (and optionally empty branches)",
		Long: "Walks the tracks state directory and removes worktree directories that have no corresponding entry in state.json. " +
			"Useful after a daemon crash or for general housekeeping. Run with --branches to also delete branches that have no commits beyond their base.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			return runGC(c.Context(), cfg)
		},
	}
	c.Flags().BoolVar(&gcPruneBranches, "branches", false, "also delete branches with no commits beyond their base (per-repo)")
	register(c)
}

func runGC(ctx context.Context, cfg config.Config) error {
	// Talk to the daemon if it's up so we observe the canonical
	// state. Fall back to reading state.json directly when offline.
	tracks, err := loadTracksForGC(cfg)
	if err != nil {
		return err
	}
	known := map[string]state.Track{}
	for _, t := range tracks {
		known[t.ID] = t
	}

	stateDir, err := cfg.ResolveStateDir()
	if err != nil {
		return err
	}
	worktreeRoot := filepath.Join(stateDir, "worktrees")
	entries, err := os.ReadDir(worktreeRoot)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	removed := 0
	quarantined := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Never touch the quarantine dir — it holds preserved unsaved work.
		if e.Name() == daemon.QuarantineDirName {
			continue
		}
		trackDir := filepath.Join(worktreeRoot, e.Name())
		track, knownByDaemon := known[e.Name()]
		if knownByDaemon && !track.Status.IsTerminal() {
			// Active track — leave alone.
			continue
		}
		// Reclaim, but never delete unsaved work: a track dir with
		// uncommitted changes or unpushed commits is moved to
		// worktrees/_recovered/<id> instead of being removed.
		wasQuarantined, reason, err := daemon.ReclaimOrphanTrackDir(ctx, worktreeRoot, e.Name())
		if err != nil {
			fmt.Printf("skip %s: %v\n", trackDir, err)
			continue
		}
		if wasQuarantined {
			quarantined++
			fmt.Printf("preserved %s (%s) — moved to worktrees/%s/%s\n", trackDir, reason, daemon.QuarantineDirName, e.Name())
		} else {
			removed++
			fmt.Printf("removed %s\n", trackDir)
		}
	}

	// Prune git worktree admin entries for every configured primary.
	for _, r := range cfg.Repos {
		path, err := r.ResolveRepoPath()
		if err != nil {
			continue
		}
		_ = git.NewPrimaryRepoClient(path).PruneWorktrees(ctx)
	}

	if gcPruneBranches {
		pruned := pruneEmptyBranches(ctx, cfg)
		fmt.Printf("pruned %d empty branches\n", pruned)
	}

	if quarantined > 0 {
		fmt.Printf("done — removed %d orphan track dir(s), preserved %d with unsaved work in worktrees/%s\n", removed, quarantined, daemon.QuarantineDirName)
	} else {
		fmt.Printf("done — removed %d orphan track dir(s)\n", removed)
	}
	return nil
}

func loadTracksForGC(cfg config.Config) ([]state.Track, error) {
	cl := daemon.NewClient(cfg)
	if tracks, err := cl.Ls(); err == nil {
		return tracks, nil
	}
	// Daemon offline — read state.json directly.
	stateDir, err := cfg.ResolveStateDir()
	if err != nil {
		return nil, err
	}
	fs, err := state.OpenFileStore(stateDir)
	if err != nil {
		return nil, err
	}
	return fs.All(), nil
}

// pruneEmptyBranches walks each configured primary and deletes
// branches named under the configured branch-types whose tip equals
// the configured base. Returns the count deleted.
func pruneEmptyBranches(ctx context.Context, cfg config.Config) int {
	count := 0
	for _, r := range cfg.Repos {
		path, err := r.ResolveRepoPath()
		if err != nil {
			continue
		}
		c := git.NewPrimaryRepoClient(path)
		// Listing branches by prefix is enough for our needs; we
		// don't have a List in PrimaryRepoClient yet so just shell
		// out directly.
		for _, t := range cfg.Branch.Types {
			out, _, err := c.Runner.Run(ctx, "for-each-ref", "--format=%(refname:short)", "refs/heads/"+t+"/")
			if err != nil {
				continue
			}
			for _, line := range splitLines(out) {
				if line == "" {
					continue
				}
				// Compare branch tip with origin/<base>. If equal,
				// it had no commits.
				baseRef, _, _ := c.Runner.Run(ctx, "rev-parse", "origin/"+r.Base)
				branchRef, _, _ := c.Runner.Run(ctx, "rev-parse", line)
				if trim(baseRef) != "" && trim(baseRef) == trim(branchRef) {
					if err := c.DeleteBranch(ctx, line); err == nil {
						fmt.Printf("deleted empty branch %s in %s\n", line, r.Name)
						count++
					}
				}
			}
		}
	}
	return count
}

func splitLines(s string) []string {
	out := []string{}
	curr := ""
	for _, r := range s {
		if r == '\n' {
			out = append(out, curr)
			curr = ""
			continue
		}
		curr += string(r)
	}
	if curr != "" {
		out = append(out, curr)
	}
	return out
}

func trim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}
