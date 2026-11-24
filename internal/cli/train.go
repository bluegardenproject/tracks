package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/spf13/cobra"
)

func newTrainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "train",
		Short: "list, show and end this track's trains, such as its reviewer",
		Long: "Trains are agents a track hands tasks to, each in a tmux session of its own that the user " +
			"opens in an overlay: for now its reviewer, which tracks review starts. Run inside a track: the " +
			"track is $TRACKS_ID, or the one whose window holds the pane.",
	}
	cmd.AddCommand(newTrainListCmd(), newTrainShowCmd(), newTrainEndCmd())
	return cmd
}

func newTrainListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list this track's trains, their agents and status",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, id, err := trainClient("train list")
			if err != nil {
				return err
			}
			trains, err := client.TrainList(c.Context(), id)
			if err != nil {
				return err
			}
			return printTrains(c.OutOrStdout(), trains)
		},
	}
}

func newTrainEndCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "end <name>",
		Short: "end one of this track's trains and close its session",
		Long:  "Ends the train tracks train list names, and closes its session. Ending the reviewer ends its review.",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if os.Getenv(review.EnvMarker) != "" {
				return fmt.Errorf("%w: a train doesn't end trains", review.ErrAlreadyReviewing)
			}
			client, id, err := trainClient("train end")
			if err != nil {
				return err
			}
			if err := client.TrainEnd(c.Context(), id, args[0]); err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Ended %s.\n", args[0])
			return err
		},
	}
}

// printTrains writes trains as a table: name, agent, what it runs on,
// status and outcome.
func printTrains(w io.Writer, trains []rpc.Train) error {
	if len(trains) == 0 {
		_, err := fmt.Fprintln(w, "No trains on this track.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tAGENT\tON\tSTATUS")
	for _, t := range trains {
		status := t.Status
		if t.Outcome != "" {
			status += ", " + t.Outcome
		}
		on := t.Engine
		if on == "" {
			on = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", t.Handle, t.Agent, on, status)
	}
	return tw.Flush()
}

// trainClient is trackClient, for the train commands the user runs too:
// in a track's terminal, where $TRACKS_ID isn't set, the track is the
// one whose window holds the pane.
func trainClient(command string) (rpc.Client, string, error) {
	id, err := trainTrack(command, "")
	if err != nil {
		return rpc.Client{}, "", err
	}
	paths, err := platform.Resolve()
	if err != nil {
		return rpc.Client{}, "", err
	}
	return rpc.Client{Socket: paths.Socket}, id, nil
}

// trainTrack is the track a train command is for: id when given, then
// $TRACKS_ID, then the track of the window holding $TMUX_PANE.
func trainTrack(command, id string) (string, error) {
	if id == "" {
		id = os.Getenv("TRACKS_ID")
	}
	if id == "" {
		paths, err := platform.Resolve()
		if err != nil {
			return "", err
		}
		if pane := ownPane(paths); pane != "" {
			id, _ = trackwin.TrackOf(tmux.New(paths.TmuxSocket), pane)
		}
	}
	if id == "" {
		return "", errNoTrack(command)
	}
	return id, nil
}

// ownPane is $TMUX_PANE when it's a pane of Tracks' tmux server, ""
// otherwise: every server numbers its panes from %0.
func ownPane(paths platform.Paths) string {
	if tmux.LocationOf(os.Getenv("TMUX"), paths.TmuxSocket) != tmux.OwnServer {
		return ""
	}
	return os.Getenv("TMUX_PANE")
}
