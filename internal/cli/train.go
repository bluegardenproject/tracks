package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/spf13/cobra"
)

func newTrainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "train",
		Short: "list and end this track's trains, such as its reviewer",
		Long: "Trains are agents a track hands tasks to, each in a hidden pane of the track: for now its " +
			"reviewer, which tracks review starts. Run inside a track: the track is $TRACKS_ID.",
	}
	cmd.AddCommand(newTrainListCmd(), newTrainEndCmd())
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
		Short: "end one of this track's trains and close its pane",
		Long:  "Ends the train tracks train list names, and closes its pane. Ending the reviewer ends its review.",
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

// trainClient is a daemon client and the track $TRACKS_ID, for command.
func trainClient(command string) (rpc.Client, string, error) {
	id := os.Getenv("TRACKS_ID")
	if id == "" {
		return rpc.Client{}, "", errNoTrack(command)
	}
	paths, err := platform.Resolve()
	if err != nil {
		return rpc.Client{}, "", err
	}
	return rpc.Client{Socket: paths.Socket}, id, nil
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
