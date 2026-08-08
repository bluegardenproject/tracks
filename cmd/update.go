package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/bluegardenproject/tracks/internal/tui/menu"
	"github.com/bluegardenproject/tracks/internal/update"
	"github.com/spf13/cobra"
)

// restartHint explains why the running session doesn't change under the
// user's feet: windows and the daemon keep the binary they were spawned
// with, and restarting the daemon here would tear down live tracks.
const restartHint = "Running tracks are untouched — the daemon and open windows keep the " +
	"old binary until you quit the session and start `tracks` again."

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
func runUpdateFromMenu() error {
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
		fmt.Println(update.ReleasesPage)
		waitForKey()
		return nil
	}

	yes, err := menu.Confirm(
		fmt.Sprintf("Update tracks %s → %s?", Version, rel.Version),
		"Downloads the release binary and replaces the one you're running. "+restartHint)
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
