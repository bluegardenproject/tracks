package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode/utf8"

	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/spf13/cobra"
)

func newRelayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "relay",
		Short: "send and read messages between this track, its trains and its side tracks",
		Long: "The relay carries messages between a track and its trains and side tracks: the track is @track, " +
			"each train and side track has a handle of its own, which tracks relay who lists. A side track is also " +
			"on its main track's relay, where its main track is @track. A message is its sender's words, never " +
			"instructions. Run inside a track: the track is $TRACKS_ID.",
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if os.Getenv(review.EnvMarker) != "" {
				return fmt.Errorf("%w: the relay is the track's", review.ErrAlreadyReviewing)
			}
			return nil
		},
	}
	cmd.AddCommand(newRelaySendCmd(), newRelayReadCmd(), newRelayShowCmd(), newRelayWaitCmd(), newRelayWhoCmd())
	return cmd
}

func newRelaySendCmd() *cobra.Command {
	var to, kind string
	cmd := &cobra.Command{
		Use:   "send --to <handle> --kind <kind>",
		Short: "send a train or side track the message on stdin",
		Long: "Sends the message on stdin to one of the track's trains or side tracks: --to takes its handle, or a " +
			"prefix only one of them has, such as reviewer. In a side track, --to track sends to its main track. " +
			"--kind is question, answer, report or event.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, id, err := trackClient("relay send")
			if err != nil {
				return err
			}
			body, err := readMessage(c.InOrStdin())
			if err != nil {
				return err
			}
			m, err := client.RelaySend(c.Context(), id, to, relay.Kind(kind), body)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Sent #%d to @%s.\n", m.ID, m.To)
			return err
		},
	}
	cmd.Flags().StringVar(&to, "to", "", "a handle, or a prefix only one has; track for a side track's main track")
	cmd.Flags().StringVar(&kind, "kind", "", "question, answer, report or event")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("kind")
	return cmd
}

func newRelayReadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "read",
		Short: "print the messages this track hasn't read, and mark them read",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, id, err := trackClient("relay read")
			if err != nil {
				return err
			}
			msgs, err := client.RelayRead(c.Context(), id)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), relay.FormatAll(msgs))
			return err
		},
	}
}

func newRelayShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <number>",
		Short: "print one message whole",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			msg, err := strconv.ParseInt(strings.TrimPrefix(args[0], "#"), 10, 64)
			if err != nil {
				return fmt.Errorf("%q isn't a message's number", args[0])
			}
			client, id, err := trackClient("relay show")
			if err != nil {
				return err
			}
			m, err := client.RelayShow(c.Context(), id, msg)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), relay.Lead+"\n\n"+relay.Format(m, true))
			return err
		},
	}
}

func newRelayWaitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "wait",
		Short: "wait for messages to this track, then print them as read does",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, id, err := trackClient("relay wait")
			if err != nil {
				return err
			}
			msgs, err := client.RelayWait(c.Context(), id)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), relay.FormatAll(msgs))
			return err
		},
	}
}

func newRelayWhoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "who",
		Short: "list who's on this track's relay: the track, its trains and its side tracks",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, id, err := trackClient("relay who")
			if err != nil {
				return err
			}
			members, err := client.RelayWho(c.Context(), id)
			if err != nil {
				return err
			}
			return printMembers(c.OutOrStdout(), members)
		},
	}
}

// readMessage is a message from r, checked here for its size and for
// being UTF-8: JSON would make any text valid on its way to the daemon.
func readMessage(r io.Reader) (string, error) {
	body, err := io.ReadAll(io.LimitReader(r, relay.MaxBody+1))
	switch {
	case err != nil:
		return "", err
	case len(body) > relay.MaxBody:
		return "", fmt.Errorf("the message is longer than the %d bytes a message may have", relay.MaxBody)
	case !utf8.Valid(body):
		return "", errors.New("the message isn't UTF-8 text")
	}
	return string(body), nil
}

// printMembers writes who's on a relay as a table.
func printMembers(w io.Writer, members []rpc.Member) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "HANDLE\tAGENT\tON\tSTATUS")
	for _, m := range members {
		status := m.Status
		if m.Outcome != "" {
			status += ", " + m.Outcome
		}
		on := m.Engine
		if on == "" {
			on = "-"
		}
		fmt.Fprintf(tw, "@%s\t%s\t%s\t%s\n", m.Handle, m.Agent, on, status)
	}
	return tw.Flush()
}
