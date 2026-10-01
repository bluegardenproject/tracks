package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/bluegardenproject/tracks/internal/update"
	"github.com/spf13/cobra"
)

// restartHint is what changes after the swap: nothing running does
// until the next `tracks`, which replaces the daemon of the old
// version. The tracks' windows and agents keep running.
const restartHint = "Tracks keeps running on the old binary until the next `tracks`, " +
	"which restarts its daemon to match. Open tracks keep running."

// releases is how the update command reaches GitHub; tests replace it.
type releases struct {
	latest func(context.Context) (update.Release, error)
	apply  func(context.Context, update.Release) (string, error)
}

var githubReleases = releases{latest: update.Latest, apply: update.Apply}

func newUpdateCmd(version string) *cobra.Command {
	var checkOnly bool
	c := &cobra.Command{
		Use:   "update",
		Short: "Check for a newer Tracks release and install it",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return runUpdate(c.Context(), c.OutOrStdout(), githubReleases, version, checkOnly)
		},
	}
	c.Flags().BoolVar(&checkOnly, "check", false, "only report whether an update is available")
	return c
}

func runUpdate(ctx context.Context, out io.Writer, r releases, version string, checkOnly bool) error {
	check, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	rel, err := r.latest(check)
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}
	if !update.Newer(version, rel.Version) {
		fmt.Fprintf(out, "tracks %s is the latest release\n", version)
		return nil
	}
	fmt.Fprintf(out, "update available: %s → %s\n", version, rel.Version)
	if checkOnly {
		fmt.Fprintln(out, rel.PageURL)
		return nil
	}
	fmt.Fprintf(out, "downloading %s %s…\n", update.AssetName(), rel.Tag)
	path, err := r.apply(ctx, rel)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "installed tracks %s at %s\n\n%s\n", rel.Version, path, restartHint)
	return nil
}
