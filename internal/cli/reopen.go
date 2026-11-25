package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
)

// reopener is what reopen needs of the daemon.
type reopener interface {
	Interrupted(ctx context.Context) ([]track.Track, error)
	Reopen(ctx context.Context, progress func(string)) ([]tracks.Reopening, error)
}

// reopen lists the tracks whose windows closed with Tracks and reopens
// them. It returns the window to show when it reopened one track alone,
// "" otherwise.
func reopen(ctx context.Context, daemon reopener, out io.Writer) (string, error) {
	interrupted, err := daemon.Interrupted(ctx)
	if err != nil || len(interrupted) == 0 {
		return "", err
	}
	fmt.Fprintf(out, "Reopening %s open when Tracks closed:\n", countTracks(len(interrupted), "that was", "that were"))
	width := 0
	for _, t := range interrupted {
		width = max(width, len(t.Name))
	}
	for _, t := range interrupted {
		fmt.Fprintf(out, "  %-*s  %s\n", width, t.Name, describe(t))
	}
	reopened, err := daemon.Reopen(ctx, func(step string) { fmt.Fprintln(out, "  "+step) })
	if err != nil {
		return "", err
	}
	var windows []string
	for _, r := range reopened {
		if r.Error != "" {
			fmt.Fprintf(out, "Couldn't reopen %s: %s\n", r.Name, r.Error)
			continue
		}
		windows = append(windows, r.Window)
	}
	if len(windows) > 0 {
		fmt.Fprintf(out, "Reopened %s.\n", countTracks(len(windows), "", ""))
	}
	if len(windows) == 1 {
		return windows[0], nil
	}
	return "", nil
}

// countTracks is "1 track" or "n tracks", followed by one or many when
// given.
func countTracks(n int, one, many string) string {
	s := fmt.Sprintf("%d tracks", n)
	verb := many
	if n == 1 {
		s, verb = "1 track", one
	}
	if verb != "" {
		s += " " + verb
	}
	return s
}

// describe is t's kind and repos, as the list of tracks to reopen shows
// them.
func describe(t track.Track) string {
	if len(t.Repos) == 0 {
		return string(t.Kind)
	}
	return string(t.Kind) + " · " + repoList(t.Repos)
}

func repoList(repos []track.Repo) string {
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	return strings.Join(names, ", ")
}
