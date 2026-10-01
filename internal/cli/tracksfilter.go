package cli

import (
	"context"
	"os"
	"strconv"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/shellx"
	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/ui/tracksfilter"
)

// filterSet is what the Tracks filter popup writes once it changed the
// filter.
const filterSet = "set"

// openTracksFilter opens the Tracks filter over client, then switches
// to the Tracks window when the filter changed.
func openTracksFilter(c *tmux.Client, paths platform.Paths, client string) error {
	command, err := selfCommand()
	if err != nil {
		return err
	}
	width, height, err := c.ClientSize(client)
	if err != nil {
		return err
	}
	version, _ := tmux.InstalledVersion()
	border := tmux.PopupBorder(version)
	t, _ := loadTheme(paths)
	result, err := os.CreateTemp("", "tracks-filter-*")
	if err != nil {
		return err
	}
	result.Close()
	defer os.Remove(result.Name())

	if err := c.Popup(tmux.Popup{
		Client:     client,
		Width:      strconv.Itoa(min(width, tracksfilter.Width+border)),
		Height:     strconv.Itoa(min(height, tracksfilter.Height()+border)),
		Command:    command + " popup tracks-filter " + shellx.Quote(result.Name()),
		Background: t.Value(theme.OverlayBg),
	}, version); err != nil {
		return err
	}
	if done, err := os.ReadFile(result.Name()); err != nil || string(done) != filterSet {
		return err
	}
	return trackwin.Switch(c, sessionName, "0", 0)
}

// tracksFilter runs the popup on the filter that's on, and gives the
// daemon what it applied or cleared.
func tracksFilter(ctx context.Context, version, resultFile string) error {
	paths, err := platform.Resolve()
	if err != nil {
		return err
	}
	t, _ := loadTheme(paths)
	c := tmux.New(paths.TmuxSocket)
	daemon := daemonCalls{tmux: c, paths: paths, version: version}
	var on track.Filter
	if err := daemon.do(ctx, func(client rpc.Client) (err error) {
		on, err = client.Filter(ctx)
		return err
	}); err != nil {
		return c.DisplayMessage("Couldn't read the tracks filter: " + err.Error())
	}
	done, err := runPopup(ctx, tracksfilter.New(t, on))
	if err != nil {
		return err
	}
	m, ok := done.(tracksfilter.Model)
	if !ok {
		return nil
	}
	action, f := m.Result()
	switch action {
	case tracksfilter.Cancel:
		return nil
	case tracksfilter.Clear:
		f = track.Filter{}
	}
	if err := daemon.do(ctx, func(client rpc.Client) error { return client.SetFilter(ctx, f) }); err != nil {
		return c.DisplayMessage("Couldn't set the tracks filter: " + err.Error())
	}
	return os.WriteFile(resultFile, []byte(filterSet), 0o600)
}
