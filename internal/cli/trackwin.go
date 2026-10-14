package cli

import (
	"context"
	"errors"
	"strconv"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/spf13/cobra"
)

// newTrackwinCmd holds the commands key bindings run. They report in
// the tmux status line, since run-shell output would cover the pane.
func newTrackwinCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "trackwin", Hidden: true}
	cmd.AddCommand(&cobra.Command{
		Use:  "add-terminal <window-id>",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			paths, err := platform.Resolve()
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
	cmd.AddCommand(&cobra.Command{
		Use:  "nav <first|prev|next|last|number> <current-window-index>",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			c := tmux.New(paths.TmuxSocket)
			current, err := strconv.Atoi(args[1])
			if err != nil {
				return c.DisplayMessage("Couldn't switch: no window index " + args[1])
			}
			if err := trackwin.Switch(c, sessionName, args[0], current); err != nil {
				return c.DisplayMessage("Couldn't switch: " + err.Error())
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:  "action <button> <window-id>",
		Args: cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			t := tmux.New(paths.TmuxSocket)
			msg := runAction(c.Context(), t, rpc.Client{Socket: paths.Socket}, args[0], args[1])
			if msg == "" {
				return nil
			}
			return t.DisplayMessage(msg)
		},
	})
	return cmd
}

// runAction runs the footer button named button in the track of window
// and words how it went; "" when the click missed every button or the
// window isn't a track's.
func runAction(ctx context.Context, t trackwin.Tmux, daemon interface {
	Action(ctx context.Context, id, action string) (string, error)
}, button, window string) string {
	if _, ok := actions.ParseRange(button); !ok {
		return ""
	}
	windows, err := trackwin.List(t, sessionName)
	if err != nil {
		return "Couldn't run the action: " + err.Error()
	}
	for _, w := range windows {
		if w.Window != window {
			continue
		}
		name, err := daemon.Action(ctx, w.Track, button)
		if err != nil {
			return err.Error()
		}
		return "Sent " + name + "'s open comments to its agent."
	}
	return ""
}
