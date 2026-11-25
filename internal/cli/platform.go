package cli

import (
	"context"
	"errors"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/train"
	uiplatform "github.com/bluegardenproject/tracks/internal/ui/platform"
	"github.com/bluegardenproject/tracks/internal/ui/style"
	"github.com/spf13/cobra"
)

// newPlatformCmd runs a track's Platform, the pane on its window's left
// that lists its trains. The daemon opens and closes it.
func newPlatformCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:    "platform",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			id := os.Getenv("TRACKS_ID")
			if id == "" {
				return errNoTrack("platform")
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			t, _ := loadTheme(paths)
			client := rpc.Client{Socket: paths.Socket}
			daemon := daemonCalls{tmux: tmux.New(paths.TmuxSocket), paths: paths, version: version}
			m := uiplatform.New(uiplatform.Config{
				Theme:     t,
				Trains:    func() ([]uiplatform.Entry, error) { return platformEntries(client.TrainList(cmd.Context(), id)) },
				Sides:     func() ([]uiplatform.Side, error) { return platformSides(client.SideList(cmd.Context(), id)) },
				OpenSide:  func(side string) error { return switchFromPlatform(daemon.tmux, paths, side) },
				Watch:     daemon.watch,
				LoadTheme: func() (theme.Theme, error) { return loadTheme(paths) },
				Collect:   func() error { return client.RelayCollect(cmd.Context(), id) },
				Open:      func(handle string) { openFromPlatform(cmd.Context(), daemon.tmux, client, paths, id, handle) },
				Signals:   func() (uiplatform.Signals, error) { return platformSignals(client.Signals(cmd.Context(), id)) },
				// Over the client showing this pane: the user's.
				OpenRelayStation: func() error { return openRelayStation(id, "") },
				Solve: func(index int) error {
					_, err := client.Action(cmd.Context(), id, actions.Range(index))
					return err
				},
			})
			_, err = tea.NewProgram(m, tea.WithContext(cmd.Context()), tea.WithColorProfile(style.Profile())).Run()
			if cmd.Context().Err() != nil {
				return nil
			}
			return err
		},
	}
}

// platformEntries are trains, from the daemon, as the Platform shows
// them.
func platformEntries(trains []rpc.Train, err error) ([]uiplatform.Entry, error) {
	if err != nil {
		return nil, err
	}
	out := make([]uiplatform.Entry, len(trains))
	for i, t := range trains {
		out[i] = uiplatform.Entry{Handle: t.Handle, Status: train.StatusOf(t.StatusID), Outcome: train.StatusOf(t.OutcomeID), Held: t.Held}
	}
	return out, nil
}

// openFromPlatform opens track id's train handle in its overlay over the
// client showing the Platform, and says over it what went wrong.
func openFromPlatform(ctx context.Context, t *tmux.Client, daemon rpc.Client, paths platform.Paths, id, handle string) {
	client, err := t.ClientOf(ownPane(paths))
	if err != nil {
		_ = t.DisplayMessage("Couldn't open " + handle + ": " + err.Error())
		return
	}
	th, _ := loadTheme(paths)
	if err := showTrain(ctx, t, daemon, th, id, handle, client); err != nil {
		_ = t.Tell(client, "Couldn't open "+handle+": "+err.Error())
	}
}

// platformSignals are Signals, from the daemon, as the Platform shows
// them.
func platformSignals(sig rpc.SignalsResult, err error) (uiplatform.Signals, error) {
	if err != nil {
		return uiplatform.Signals{}, err
	}
	out := uiplatform.Signals{RelayNew: sig.RelayNew, Held: sig.Held}
	for _, pr := range sig.PRs {
		out.PRs = append(out.PRs, uiplatform.PR{Index: pr.Index, PR: pr.PR, Action: pr.Action, Sent: pr.Sent})
	}
	return out, nil
}

// platformSides are side tracks, from the daemon, as the Platform shows
// them: their badges the Station's, with its labels.
func platformSides(sides []rpc.SideTrack, err error) ([]uiplatform.Side, error) {
	if err != nil {
		return nil, err
	}
	out := make([]uiplatform.Side, len(sides))
	for i, s := range sides {
		out[i] = uiplatform.Side{ID: s.ID, Name: s.Name, Status: track.StatusByID(s.StatusID)}
		if s.PRID != "" {
			out[i].PR = track.PRStatusByID(s.PRID)
			out[i].PR.Label = s.PR
		}
	}
	return out, nil
}

// switchFromPlatform switches the client showing the Platform to side
// track id's window.
func switchFromPlatform(t *tmux.Client, paths platform.Paths, id string) error {
	client, err := t.ClientOf(ownPane(paths))
	if err != nil {
		return err
	}
	infos, err := trackwin.List(t, sessionName)
	if err != nil {
		return err
	}
	for _, in := range infos {
		if in.Track == id {
			return t.SwitchClient(client, in.Window)
		}
	}
	return errors.New("its window is closed: open it from the Station")
}
