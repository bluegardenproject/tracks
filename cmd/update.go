package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tui/menu"
	"github.com/bluegardenproject/tracks/internal/update"
	"github.com/spf13/cobra"
)

// restartHint spells out what actually happens after the swap. The
// update itself changes nothing that's already running — but the next
// bare `tracks` invocation sees a daemon older than the CLI and restarts
// it (see daemonStaleReason), which interrupts every live track. Updating
// mid-session is therefore safe; the next `tracks` is the disruptive part,
// and the user should hear that before, not after.
const restartHint = "The swap leaves the daemon and open windows on the old binary. The next " +
	"`tracks` run restarts the daemon to match, interrupting any track still " +
	"running — bring those back with Reopen."

func init() {
	var checkOnly bool
	c := &cobra.Command{
		Use:   "update",
		Short: "check for a newer tracks release and install it",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			rel, available, err := checkForUpdate(c.Context())
			if err != nil {
				return err
			}
			if !available {
				fmt.Printf("tracks %s is the latest release\n", Version)
				return nil
			}
			fmt.Printf("update available: %s → %s\n", Version, rel.Version)
			if checkOnly {
				fmt.Println(rel.PageURL)
				return nil
			}
			return applyUpdate(c.Context(), rel)
		},
	}
	c.Flags().BoolVar(&checkOnly, "check", false, "only report whether an update is available")
	register(c)
}

// checkForUpdate fetches the latest release and reports whether it's
// newer than this binary.
func checkForUpdate(ctx context.Context) (update.Release, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	rel, err := update.Latest(ctx)
	if err != nil {
		return update.Release{}, false, fmt.Errorf("checking for updates: %w", err)
	}
	return rel, update.Newer(Version, rel.Version), nil
}

func applyUpdate(ctx context.Context, rel update.Release) error {
	if ctx == nil {
		ctx = context.Background()
	}
	fmt.Printf("downloading %s %s…\n", update.AssetName(), rel.Tag)
	path, err := update.Apply(ctx, rel)
	if err != nil {
		return err
	}
	fmt.Printf("installed tracks %s at %s\n", rel.Version, path)
	fmt.Println()
	fmt.Println(restartHint)
	return nil
}

// runUpdateFromMenu is the menu-side flow: check, show what's on offer,
// and install it after an explicit confirm. Errors are printed rather
// than returned so the popup shows them instead of closing on the spot.
func runUpdateFromMenu(cfg config.Config) error {
	fmt.Println("checking for updates…")
	rel, available, err := checkForUpdate(context.Background())
	if err != nil {
		fmt.Println(err)
		waitForKey()
		return nil
	}
	if !available {
		fmt.Printf("tracks %s is the latest release.\n", Version)
		waitForKey()
		return nil
	}
	if rel.AssetURL == "" {
		fmt.Printf("tracks %s is out, but the release has no %s binary.\n", rel.Version, update.AssetName())
		page := rel.PageURL
		if page == "" {
			page = update.ReleasesPage
		}
		fmt.Println(page)
		waitForKey()
		return nil
	}

	body := "Downloads the release binary and replaces the one you're running. " + restartHint
	if n := liveTrackCount(cfg); n > 0 {
		body += fmt.Sprintf(" %d track(s) are running right now.", n)
	}
	yes, err := menu.Confirm(
		fmt.Sprintf("Update tracks %s → %s?", Version, rel.Version), body)
	if err != nil || !yes {
		return nil
	}
	fmt.Println()
	if err := applyUpdate(context.Background(), rel); err != nil {
		fmt.Println(err)
	}
	waitForKey()
	return nil
}

// liveTrackCount is how many tracks the daemon restart would interrupt.
// The predicate mirrors the daemon's own shutdown sweep (sweepable in
// internal/daemon/recovery.go), so the number matches what the warning
// claims: a pr-open track is re-adopted on the next start, not
// interrupted. A daemon we can't reach reports none — the count only
// sharpens the warning, it isn't worth failing the update over.
func liveTrackCount(cfg config.Config) int {
	tracks, err := daemon.NewClient(cfg).Ls()
	if err != nil {
		return 0
	}
	n := 0
	for _, t := range tracks {
		if !t.Status.IsTerminal() && t.Status != state.StatusDraft && t.Status != state.StatusPROpen {
			n++
		}
	}
	return n
}
