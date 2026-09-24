//go:build v2

package main

import (
	"context"
	"os"

	v2cli "github.com/bluegardenproject/tracks/internal/v2/cli"
)

func runNewApp(ctx context.Context, args []string) error {
	if err := os.Setenv(newAppEnv, "1"); err != nil {
		return err
	}
	return v2cli.Execute(ctx, args, Version)
}
