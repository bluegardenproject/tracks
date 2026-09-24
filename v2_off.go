//go:build !v2

package main

import (
	"context"
	"errors"
)

// runNewApp refuses in release builds, which contain no v2 code. This
// also stops an installed tracks from acting inside a v2 dev session,
// where TRACKS_NEW_APP=1 is set.
func runNewApp(context.Context, []string) error {
	return errors.New("this build has no Tracks v2 (--new-app or TRACKS_NEW_APP=1).\n" +
		"  Build it from the tracks repo: make dev && ./tracks --new-app")
}
