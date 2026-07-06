package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tui/menu"
	"github.com/bluegardenproject/tracks/internal/tui/newtrack"
	"github.com/bluegardenproject/tracks/internal/tui/settings"
	"github.com/spf13/cobra"
)

func init() {
	register(&cobra.Command{
		Use:   "menu",
		Short: "open the overlay menu (bound to <prefix>+<menu_key> inside the tmux session)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			cfg, _ := config.Load()
			action, err := menu.PickAction()
			if err != nil {
				if errors.Is(err, menu.ErrCancelled) {
					return nil
				}
				return err
			}
			return runMenuAction(cfg, action)
		},
	})
}

func runMenuAction(cfg config.Config, action menu.Action) error {
	cl := daemon.NewClient(cfg)
	tm := tmux.New()

	switch action {
	case menu.ActionClose:
		return nil

	case menu.ActionNewTrack:
		return runNewTrackFromMenu(cfg)

	case menu.ActionDashboard:
		// Ensure the dashboard window exists, then switch to it.
		return ensureWindowAndSelect(cfg, tm, "Dashboard", "dashboard")

	case menu.ActionList:
		// `ls` is a one-shot dump — show it inside the popup so the
		// user can read it before the popup closes.
		tracks, err := cl.Ls()
		if err != nil {
			return err
		}
		if len(tracks) == 0 {
			fmt.Println("no tracks yet")
			waitForKey()
			return nil
		}
		for _, t := range tracks {
			fmt.Printf("  %-15s  %-30s  %-10s\n", lastN(t.ID, 15), t.Branch, t.Status)
		}
		waitForKey()
		return nil

	case menu.ActionAttach:
		t, err := menu.PickTrack(cl, "Attach to which track?", nil)
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			if errors.Is(err, menu.ErrNoTracks) {
				fmt.Println(err)
				waitForKey()
				return nil
			}
			return err
		}
		window := t.WindowName()
		exists, _ := tm.HasWindow(cfg.Tmux.SessionName, window)
		if !exists {
			self, _ := selfBinary()
			cmdLine := fmt.Sprintf("%s log %s", shellQuote(self), shellQuote(t.ID))
			if err := tm.NewWindow(cfg.Tmux.SessionName, window, cmdLine, "", true); err != nil {
				return err
			}
		}
		return tm.SelectWindow(cfg.Tmux.SessionName, window)

	case menu.ActionDone:
		t, err := menu.PickTrack(cl, "End which track?", menu.ActiveOnly)
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			if errors.Is(err, menu.ErrNoTracks) {
				fmt.Println(err)
				waitForKey()
				return nil
			}
			return err
		}
		if err := cl.Done(t.ID); err != nil {
			return err
		}
		fmt.Printf("done: %s\n", t.ID)
		waitForKey()
		return nil

	case menu.ActionKill:
		t, err := menu.PickTrack(cl, "Kill which track?", menu.ActiveOnly)
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			if errors.Is(err, menu.ErrNoTracks) {
				fmt.Println(err)
				waitForKey()
				return nil
			}
			return err
		}
		if err := cl.Kill(t.ID); err != nil {
			return err
		}
		fmt.Printf("killed: %s\n", t.ID)
		waitForKey()
		return nil

	case menu.ActionAddRepo:
		t, err := menu.PickTrack(cl, "Add a repo to which track?", menu.WorktreeTrack)
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			if errors.Is(err, menu.ErrNoTracks) {
				fmt.Println("no active worktree tracks — ask/plan tracks must be promoted first")
				waitForKey()
				return nil
			}
			return err
		}
		exclude := map[string]bool{}
		for _, r := range t.Repos {
			exclude[r.Name] = true
		}
		repoName, err := menu.PickConfigRepo(cfg, exclude, "Add which repo?")
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			if errors.Is(err, menu.ErrNoRepos) {
				fmt.Println("every configured repo is already in this track")
				waitForKey()
				return nil
			}
			return err
		}
		fmt.Printf("adding %s to %s...\n\n", repoName, lastN(t.ID, 15))
		res, err := cl.AddRepoWithProgress(daemon.AddRepoParams{TrackID: t.ID, RepoName: repoName}, func(msg string) {
			fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
		})
		if err != nil {
			fmt.Println()
			fmt.Println("daemon:", err)
			waitForKey()
			return nil
		}
		fmt.Printf("\nadded worktree at %s\n", res.WorktreePath)
		waitForKey()
		return nil

	case menu.ActionPromote:
		t, err := menu.PickTrack(cl, "Promote which read-only track?", menu.PromotableOnly)
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			if errors.Is(err, menu.ErrNoTracks) {
				fmt.Println("no read-only ask/plan tracks to promote")
				waitForKey()
				return nil
			}
			return err
		}
		fmt.Printf("promoting %s...\n\n", lastN(t.ID, 15))
		res, err := cl.PromoteWithProgress(t.ID, func(msg string) {
			fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
		})
		if err != nil {
			fmt.Println()
			fmt.Println("daemon:", err)
			waitForKey()
			return nil
		}
		if tm.HasSession(cfg.Tmux.SessionName) && res.WindowName != "" {
			_ = tm.SelectWindow(cfg.Tmux.SessionName, res.WindowName)
		}
		fmt.Printf("\npromoted to a work track on branch %s\n", res.Branch)
		waitForKey()
		return nil

	case menu.ActionReleaseBranch:
		t, err := menu.PickTrack(cl, "Release which branch back to your repo?", menu.HasLiveWorktree)
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			if errors.Is(err, menu.ErrNoTracks) {
				fmt.Println(err)
				waitForKey()
				return nil
			}
			return err
		}
		fmt.Printf("releasing %s...\n\n", t.Branch)
		if err := cl.DoneWithProgress(t.ID, func(msg string) {
			fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
		}); err != nil {
			fmt.Println()
			fmt.Println("daemon: ", err)
			waitForKey()
			return nil
		}
		fmt.Println()
		fmt.Printf("released %s — track %s ended, worktree removed.\n", t.Branch, t.ID)
		fmt.Println()
		printCheckoutHints(cfg, t)
		waitForKey()
		return nil

	case menu.ActionForget:
		t, err := menu.PickTrack(cl, "Forget which completed track?", menu.CompletedOnly)
		if err != nil {
			if errors.Is(err, menu.ErrCancelled) {
				return nil
			}
			if errors.Is(err, menu.ErrNoTracks) {
				fmt.Println(err)
				waitForKey()
				return nil
			}
			return err
		}
		if err := cl.Forget(t.ID); err != nil {
			return err
		}
		fmt.Printf("forgot %s\n", t.ID)
		waitForKey()
		return nil

	case menu.ActionPrune:
		yes, err := menu.Confirm("Clear all completed tracks?",
			"Removes every done/errored track from the dashboard. Worktrees are already gone; branches and log files stay on disk.")
		if err != nil || !yes {
			return nil
		}
		n, err := cl.PruneCompleted()
		if err != nil {
			return err
		}
		fmt.Printf("cleared %d completed track(s)\n", n)
		waitForKey()
		return nil

	case menu.ActionProxy:
		result, err := cl.ProxyStatus()
		if err != nil {
			fmt.Println("daemon:", err)
			waitForKey()
			return nil
		}
		if len(result.Proxies) == 0 {
			fmt.Println("no proxy_port configured in any service")
			fmt.Println("add proxy_port: <N> to a service in ~/.config/tracks/config.yaml")
			waitForKey()
			return nil
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SERVICE\tFIXED PORT\tUPSTREAM\tACTIVE TRACK")
		for _, p := range result.Proxies {
			upstream := p.Upstream
			if upstream == "" {
				upstream = "(none — 503)"
			}
			trackID := p.ActiveTrackID
			if trackID == "" && p.Upstream != "" {
				trackID = "(unknown)"
			}
			fmt.Fprintf(tw, "%s\t:%d\t%s\t%s\n", p.ServiceName, p.PublicPort, upstream, trackID)
		}
		_ = tw.Flush()
		waitForKey()
		return nil

	case menu.ActionSettings:
		// Load config WITH its error so settings.Run can back up a
		// broken file before saving a fresh one. The cfg passed in
		// has any defaults already filled by the outer
		// runMenuAction caller.
		freshCfg, loadErr := config.Load()
		return settings.Run(freshCfg, loadErr)

	case menu.ActionGC:
		fmt.Println("running tracks gc...")
		if err := runGC(context.Background(), cfg); err != nil {
			return err
		}
		waitForKey()
		return nil

	case menu.ActionQuitSession:
		yes, err := menu.ConfirmQuit(cfg.Tmux.SessionName)
		if err != nil || !yes {
			return nil
		}
		return tm.KillSession(cfg.Tmux.SessionName)
	}
	return nil
}

// runNewTrackFromMenu is a wrapper that runs the same picker as
// `tracks new` from inside the popup, then asks the daemon and
// opens the per-track window.
//
// We never let an error escape silently here: the popup is `-E`,
// meaning it closes when this function returns. If we just returned
// an error to cobra, the popup would vanish and the user would see
// no feedback. Print + waitForKey on failure so the user can read
// what happened before the popup closes.
func runNewTrackFromMenu(cfg config.Config) error {
	params, err := newtrack.Run(cfg)
	if err != nil {
		if errors.Is(err, newtrack.ErrCancelled) {
			return nil
		}
		fmt.Println("error running picker:", err)
		waitForKey()
		return nil
	}
	cl := daemon.NewClient(cfg)
	// Long-running step — stream progress so the popup shows the
	// fetch / worktree-add / spawn steps as they happen instead of
	// looking frozen.
	fmt.Println("creating track...")
	fmt.Println()
	res, err := cl.NewWithProgress(params, func(msg string) {
		fmt.Printf("  [%s] %s\n", time.Now().Format("15:04:05"), msg)
	})
	if err != nil {
		fmt.Println()
		fmt.Println("daemon refused track:", err)
		fmt.Println()
		fmt.Println("If this looks like a protocol mismatch, the daemon may be running an")
		fmt.Println("older binary. Run `tmux kill-session -t tracks && tracks` to refresh.")
		waitForKey()
		return nil
	}
	tm := tmux.New()
	if tm.HasSession(cfg.Tmux.SessionName) && res.WindowName != "" {
		_ = tm.SelectWindow(cfg.Tmux.SessionName, res.WindowName)
	}
	fmt.Printf("created %s on %s\n", res.TrackID, res.Branch)
	return nil
}

// ensureWindowAndSelect creates the window if missing (running the
// supplied default command) and selects it.
func ensureWindowAndSelect(cfg config.Config, tm *tmux.Client, window, command string) error {
	exists, err := tm.HasWindow(cfg.Tmux.SessionName, window)
	if err != nil {
		return err
	}
	if !exists {
		self, _ := selfBinary()
		full := fmt.Sprintf("%s %s", shellQuote(self), command)
		if err := tm.NewWindow(cfg.Tmux.SessionName, window, full, "", true); err != nil {
			return err
		}
	}
	return tm.SelectWindow(cfg.Tmux.SessionName, window)
}

// printCheckoutHints prints a one-line `git checkout` command per
// participating repo, using the actual branch the worktree was on
// at the time of release. So a multi-repo track gets one line per
// repo, each pointing at the right path + branch.
func printCheckoutHints(cfg config.Config, t state.Track) {
	if t.Branch == "" && len(t.Repos) == 0 {
		return
	}
	fmt.Println("To check the branch out in your primary checkout:")
	for _, tr := range t.Repos {
		repo, ok := cfg.RepoByName(tr.Name)
		if !ok {
			continue
		}
		path, _ := repo.ResolveRepoPath()
		branch := tr.Branch
		if branch == "" {
			branch = t.Branch
		}
		fmt.Printf("  git -C %s checkout %s\n", path, branch)
	}
}

// waitForKey blocks until the user presses any key. Used after a
// menu action that prints output, so the popup doesn't vanish before
// the user can read it.
func waitForKey() {
	fmt.Print("\npress enter to close…")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
}

func lastN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
