package cli

import (
	"context"
	"errors"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/ui/relaystation"
	"github.com/bluegardenproject/tracks/internal/ui/style"
	"github.com/spf13/cobra"
)

// relayStationSize is the Relay Station popup's share of the client,
// as a train's overlay.
const relayStationSize = "80%"

// relayStationReconnect is how long the Relay Station waits to follow
// the daemon again once its change stream broke.
const relayStationReconnect = time.Second

func newRelayStationCmd(version string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:    "relay-station",
		Short:  "show the track's relay, as the user reads it",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if id == "" {
				return errNoTrack("relay-station")
			}
			return runRelayStation(c.Context(), version, id)
		},
	}
	cmd.PersistentFlags().StringVar(&id, "track", os.Getenv("TRACKS_ID"), "the track, $TRACKS_ID by default")
	var client string
	open := &cobra.Command{
		Use:   "open",
		Short: "open the Relay Station over a tmux client",
		Long: "Opens the track's Relay Station in a popup over --client, by default the client showing this pane. " +
			"Esc or q closes it.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			if id == "" {
				return errNoTrack("relay-station open")
			}
			return openRelayStation(id, client)
		},
	}
	open.Flags().StringVar(&client, "client", "", "the tmux client to open it over")
	cmd.AddCommand(open)
	return cmd
}

// openRelayStation opens track id's Relay Station over client, and
// returns once it's closed.
func openRelayStation(id, client string) error {
	paths, err := platform.Resolve()
	if err != nil {
		return err
	}
	command, err := selfCommand()
	if err != nil {
		return err
	}
	c := tmux.New(paths.TmuxSocket)
	if client == "" {
		if client, err = c.ClientOf(os.Getenv("TMUX_PANE")); err != nil {
			return err
		}
	}
	t, _ := loadTheme(paths)
	return c.Popup(tmux.Popup{
		Client:     client,
		Width:      relayStationSize,
		Height:     relayStationSize,
		Command:    command + " relay-station --track " + shellx.Quote(id),
		Background: t.Value(theme.OverlayBg),
	})
}

// runRelayStation runs the Relay Station on track id, reading its relay
// again after each change the daemon tells of.
func runRelayStation(ctx context.Context, version, id string) error {
	paths, err := platform.Resolve()
	if err != nil {
		return err
	}
	t, _ := loadTheme(paths)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	p := tea.NewProgram(relaystation.New(t, time.Now), tea.WithContext(ctx), tea.WithColorProfile(style.Profile()))
	daemon := daemonCalls{tmux: tmux.New(paths.TmuxSocket), paths: paths, version: version}
	go func() {
		for ctx.Err() == nil {
			err := daemon.watch(ctx, func() {
				r, err := rpc.Client{Socket: paths.Socket}.RelayStation(ctx, id)
				p.Send(relaystation.Loaded{Messages: r.Messages, Waiting: r.Waiting, Err: err})
			})
			if ctx.Err() != nil {
				return
			}
			if err == nil {
				err = errors.New("the daemon stopped; connecting again")
			}
			p.Send(relaystation.Loaded{Err: err})
			select {
			case <-ctx.Done():
			case <-time.After(relayStationReconnect):
			}
		}
	}()
	_, err = p.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}
