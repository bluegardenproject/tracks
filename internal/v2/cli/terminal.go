package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
	"github.com/spf13/cobra"
)

// errNoTrack is the error of a track command run outside a track.
func errNoTrack(command string) error {
	return fmt.Errorf("tracks %s works inside a track: $TRACKS_ID isn't set", command)
}

func newTerminalCmd() *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "terminal",
		Short: "open a terminal pane in a track's window",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if id == "" {
				id = os.Getenv("TRACKS_ID")
			}
			if id == "" {
				return errNoTrack("terminal")
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			name, err := openTerminal(tmux.New(paths.TmuxSocket), sessionName, id)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Opened a terminal in %s.\n", name)
			return err
		},
	}
	cmd.Flags().StringVar(&id, "track", "", "the track's ID (default $TRACKS_ID)")
	return cmd
}

// openTerminal adds a terminal to the window of the track with id and
// returns the track's name.
func openTerminal(t trackwin.Tmux, session, id string) (string, error) {
	windows, err := trackwin.List(t, session)
	if err != nil {
		return "", err
	}
	for _, w := range windows {
		if w.Track != id {
			continue
		}
		if _, err := trackwin.AddTerminal(t, w.Window); err != nil {
			if errors.Is(err, trackwin.ErrNotTrackWindow) {
				return "", fmt.Errorf("%s's window has no working directory to open a terminal in", w.Name)
			}
			return "", fmt.Errorf("couldn't open a terminal in %s: %w", w.Name, err)
		}
		return w.Name, nil
	}
	return "", fmt.Errorf("track %s has no open window", id)
}
