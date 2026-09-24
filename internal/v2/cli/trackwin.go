package cli

import (
	"errors"

	"github.com/bluegardenproject/tracks/internal/v2/platform"
	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/trackwin"
	"github.com/spf13/cobra"
)

// newTrackwinCmd holds the commands key bindings run. They report in
// the tmux status line, since run-shell output would cover the pane.
func newTrackwinCmd(profile profileFunc) *cobra.Command {
	cmd := &cobra.Command{Use: "trackwin", Hidden: true}
	cmd.AddCommand(&cobra.Command{
		Use:  "add-terminal <window-id>",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			paths, err := platform.Resolve(profile())
			if err != nil {
				return err
			}
			c := tmux.New(paths.TmuxSocket)
			_, err = trackwin.AddTerminal(c, args[0])
			switch {
			case errors.Is(err, trackwin.ErrNotTrackWindow):
				return c.DisplayMessage("Terminals open in track windows.")
			case err != nil:
				return c.DisplayMessage("Couldn't add a terminal: " + err.Error())
			}
			return nil
		},
	})
	return cmd
}
