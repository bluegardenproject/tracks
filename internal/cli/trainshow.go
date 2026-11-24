package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/train"
	"github.com/spf13/cobra"
)

// overlaySize is a train's overlay's width and height, of the client's.
const overlaySize = "90%"

func newTrainShowCmd() *cobra.Command {
	var id, client string
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "open one of this track's trains in an overlay",
		Long: "Opens the train tracks train list names in an overlay over the track: its own session, " +
			"to answer it, ask it things or scroll. Hide, or Ctrl+b Esc, closes the overlay and the train runs " +
			"on; End train ends it. It returns once the overlay closes. For the user: an agent doesn't run it.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if os.Getenv(review.EnvMarker) != "" {
				return fmt.Errorf("%w: a train doesn't open trains", review.ErrAlreadyReviewing)
			}
			id, err := trainTrack("train show", id)
			if err != nil {
				return err
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			t := tmux.New(paths.TmuxSocket)
			if client == "" {
				pane := ownPane(paths)
				if pane == "" {
					return errors.New("no client to open the overlay over: run it in a pane of Tracks, or pass --client")
				}
				if client, err = t.ClientOf(pane); err != nil {
					return fmt.Errorf("no client to open the overlay over: %w", err)
				}
			}
			th, _ := loadTheme(paths)
			return showTrain(c.Context(), t, rpc.Client{Socket: paths.Socket}, th, id, args[0], client)
		},
	}
	cmd.Flags().StringVar(&id, "track", "", "the track's ID (default $TRACKS_ID)")
	cmd.Flags().StringVar(&client, "client", "", "the tmux client to open it over (default the one showing this pane)")
	return cmd
}

// showTrain opens track id's train name in an overlay over client: a
// popup with a client of its own attached to the train's session. It
// returns once the popup closes.
func showTrain(ctx context.Context, t *tmux.Client, daemon interface {
	TrainList(ctx context.Context, id string) ([]rpc.Train, error)
}, th theme.Theme, id, name, client string) error {
	trains, err := daemon.TrainList(ctx, id)
	if err != nil {
		return err
	}
	handles := make([]string, len(trains))
	for i, tr := range trains {
		handles[i] = tr.Handle
	}
	handle, err := train.Resolve(name, handles)
	if err != nil {
		return fmt.Errorf("%v: tracks train list lists them", err)
	}
	session := trackwin.TrainSession(id, handle)
	if !t.HasSession(session) {
		return fmt.Errorf("%s has no session: it may have just ended", handle)
	}
	return t.Popup(trainPopup(t.Socket(), client, session, th))
}

// trainPopup is the popup over client attached to session on the tmux
// server on socket. The client inside is a client of that server of
// its own, so $TMUX goes: tmux won't nest otherwise.
func trainPopup(socket, client, session string, th theme.Theme) tmux.Popup {
	return tmux.Popup{
		Client: client, Width: overlaySize, Height: overlaySize,
		Command: "env -u TMUX tmux -L " + shellx.Quote(socket) + " attach-session -t " + shellx.Quote("="+session),
		Border:  th.Value(theme.OverlayBorder),
	}
}
