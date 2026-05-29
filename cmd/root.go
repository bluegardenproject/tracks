// Package cmd defines the cobra command tree for the `tracks` binary.
//
// Each subcommand lives in its own file (new.go, ls.go, daemon.go, …)
// and registers itself via init() using the register() helper. The
// root command itself only owns global flags and pre-run plumbing,
// keeping subcommands focused on a single feature each.
//
// This mirrors the layout of the sibling CLIs `stac-man` and
// `github-butler` so the three feel uniform when used together.
package cmd

import (
	"context"

	"github.com/spf13/cobra"
)

// Version and BuildTime are set by main.SetVersion at process start.
// They live in package main so Release Please's `extra-files` config
// can rewrite the literals without touching cmd/.
var (
	Version   = "dev"
	BuildTime = "unknown"
)

// SetVersion is called from main() with the ldflags-injected values
// before the cobra tree runs. Keeping it a setter (rather than reading
// a package-main variable directly) lets cmd/ stay free of an import
// cycle on the parent module.
func SetVersion(version, buildTime string) {
	if version != "" {
		Version = version
	}
	if buildTime != "" {
		BuildTime = buildTime
	}
}

// Global flags exposed on the root command. Subcommands read these via
// the package-level vars below rather than re-declaring them.
var (
	flagNoColor bool
	flagVerbose bool
)

// pendingSubcommands is populated by each subcommand's init() via the
// register() helper. We attach them in newRootCmd so init() ordering
// between files doesn't matter.
var pendingSubcommands []*cobra.Command

// register adds a subcommand to be wired into the root in newRootCmd.
// Files in this package call register(...) from init() to keep
// per-command wiring close to the command itself.
func register(c *cobra.Command) {
	pendingSubcommands = append(pendingSubcommands, c)
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "tracks",
		Short:         "tracks — parallel Claude agents over git worktrees",
		Long:          "tracks spins up multiple Claude Code agents in isolated git worktrees and coordinates them through a single tmux session — without ever touching the user's primary checkout.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		// Bare `tracks` invocation: bootstrap the tmux session if it
		// doesn't exist, ensure the daemon is up, then attach.
		RunE: func(c *cobra.Command, args []string) error {
			return bootstrap(c.Context())
		},
	}

	root.PersistentFlags().BoolVar(&flagNoColor, "no-color", false, "disable all color output (also respects NO_COLOR env var)")
	root.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output (prints underlying git/tmux commands)")

	for _, sub := range pendingSubcommands {
		root.AddCommand(sub)
	}

	return root
}

// Execute runs the root command. Cancellation from ctx is passed through
// to subcommands via cobra's SetContext so a Ctrl+C tears down cleanly.
func Execute(ctx context.Context) error {
	root := newRootCmd()
	root.SetContext(ctx)
	return root.Execute()
}

// Root returns a freshly-built command tree without executing it.
// Useful for tooling that needs to introspect the cobra surface.
// Each call returns a new tree; callers should not assume identity.
func Root() *cobra.Command {
	return newRootCmd()
}
