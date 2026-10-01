package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/tracks"
)

// reopener is what offerReopen needs of the daemon.
type reopener interface {
	Interrupted(ctx context.Context) ([]track.Track, error)
	Reopen(ctx context.Context, progress func(string)) ([]tracks.Reopening, error)
}

// offerReopen lists the tracks whose windows closed with Tracks and
// asks on in whether to reopen them, as v1 does when its session
// starts. It returns the window to show when it reopened one track
// alone, "" otherwise. No leaves them as they are, to be asked again.
func offerReopen(ctx context.Context, daemon reopener, in io.Reader, out io.Writer) (string, error) {
	interrupted, err := daemon.Interrupted(ctx)
	if err != nil || len(interrupted) == 0 {
		return "", err
	}
	fmt.Fprintf(out, "%s open when Tracks closed:\n", countTracks(len(interrupted), "was", "were"))
	width := 0
	for _, t := range interrupted {
		width = max(width, len(t.Name))
	}
	for _, t := range interrupted {
		fmt.Fprintf(out, "  %-*s  %s\n", width, t.Name, describe(t))
	}
	them := "them"
	if len(interrupted) == 1 {
		them = "it"
	}
	fmt.Fprintf(out, "Reopen %s? [Y/n] ", them)
	answer, err := bufio.NewReader(in).ReadString('\n')
	if err != nil {
		fmt.Fprintln(out)
		answer = "n"
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "y", "yes":
	default:
		fmt.Fprintln(out, "Left as they are: resume them from Station, or answer again when Tracks next starts.")
		return "", nil
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

// describe is t's kind and repos, as the list before the question shows
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

// stdinIsTerminal says a question can be asked on stdin.
func stdinIsTerminal() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
