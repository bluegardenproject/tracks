package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/trackwin"
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
			name, pane, err := openTerminal(tmux.New(paths.TmuxSocket), sessionName, id)
			if err != nil {
				return err
			}
			number := strings.TrimPrefix(pane, "%")
			msg := fmt.Sprintf("Opened terminal %s in %s", number, name)
			if os.Getenv("TRACKS_ID") == id {
				msg += "; tracks close " + number + " closes it"
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), msg+".")
			return err
		},
	}
	cmd.Flags().StringVar(&id, "track", "", "the track's ID (default $TRACKS_ID)")
	return cmd
}

// openTerminal adds a terminal to the window of the track with id and
// returns the track's name and the terminal's pane.
func openTerminal(t trackwin.Tmux, session, id string) (name, pane string, err error) {
	w, err := trackWindow(t, session, id)
	if err != nil {
		return "", "", err
	}
	pane, err = trackwin.AddTerminal(t, w.Window)
	if errors.Is(err, trackwin.ErrNotTrackWindow) {
		return "", "", fmt.Errorf("%s's window has no working directory to open a terminal in", w.Name)
	} else if err != nil {
		return "", "", fmt.Errorf("couldn't open a terminal in %s: %w", w.Name, err)
	}
	return w.Name, pane, nil
}
