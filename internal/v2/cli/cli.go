// Package cli is the command tree of the Tracks v2 app, reached with
// `tracks --new-app` in builds tagged v2 (`make dev`). See
// docs/v2/masterplan.md.
package cli

import (
	"context"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/spf13/cobra"
)

// Execute runs the v2 command tree with args (the flag already removed).
func Execute(ctx context.Context, args []string, version string) error {
	root := newRoot(version)
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

func newRoot(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "tracks --new-app",
		Short:         "Tracks v2 (dev build)",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return start(platform.Default)
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(newPathsCmd(), newStopCmd(), newTracksWindowCmd(version))
	return root
}
