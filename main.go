// Command tracks runs coding agents in parallel over git worktrees,
// coordinated from one tmux session. Each track is one agent working on
// a branch of its own in an isolated worktree, so the user's primary
// checkout is never disturbed.
//
// See README.md for the overall design.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bluegardenproject/tracks/internal/cli"
)

// Version is the binary version, set at build time via:
//
//	-ldflags "-X main.Version=v1.2.3"
//
// Release Please bumps this on every release via the `extra-files`
// entry in release-please-config.json, so the in-tree default also
// matches the latest tagged release between rebuilds.
var Version = "1.3.1" // x-release-please-version

// BuildTime is the UTC timestamp the binary was built at, set via:
//
//	-ldflags "-X main.BuildTime=2026-05-29T17:00:00Z"
var BuildTime = "unknown"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	_, args := newAppRequested(os.Args[1:], os.Getenv)
	if err := os.Setenv(newAppEnv, "1"); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := cli.Execute(ctx, args, Version, BuildTime); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
