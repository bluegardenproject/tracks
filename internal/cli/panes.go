package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/procs"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/spf13/cobra"
)

func newPanesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "panes",
		Short: "list the panes beside this track's agent: terminals, dev servers, setup",
		Long: "Lists the panes of the track window's right-hand column, each with the number tracks close " +
			"takes, what runs in it and the ports it listens on. Run inside a track: the track is $TRACKS_ID.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			t, w, err := currentTrackWindow("panes")
			if err != nil {
				return err
			}
			return listPanes(c.Context(), c.OutOrStdout(), t, w)
		},
	}
}

// listPanes prints the panes beside w's agent, as tracks panes does.
func listPanes(ctx context.Context, out io.Writer, t *tmux.Client, w trackwin.Info) error {
	column, err := trackPanes(t, w)
	if err != nil {
		return err
	}
	if len(column) == 0 {
		_, err := fmt.Fprintln(out, "No panes beside the agent.")
		return err
	}
	snap, err := procs.Take(ctx)
	if err != nil {
		snap = procs.New(nil, nil)
	}
	return printPanes(out, w.Window, column, snap)
}

func newCloseCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "close <pane>...",
		Short: "close panes beside this track's agent, ending what runs in them",
		Long: "Closes panes of the track window's right-hand column, by the numbers tracks panes lists: what " +
			"runs in each gets SIGTERM, then SIGKILL if it hasn't ended after a few seconds. A dev server's pane " +
			"closes as tracks down closes it. The agent's own pane is never closed, nor a setup that still runs. " +
			"Run inside a track: the track is $TRACKS_ID.",
		RunE: func(c *cobra.Command, args []string) error {
			if all == (len(args) > 0) {
				return fmt.Errorf("name the panes to close, or pass --all")
			}
			t, w, err := currentTrackWindow("close")
			if err != nil {
				return err
			}
			column, err := trackPanes(t, w)
			if err != nil {
				return err
			}
			targets, err := pickPanes(column, args)
			if err != nil {
				return err
			}
			closeErr := closePanes(c.Context(), c.OutOrStdout(), t, w, targets)
			if err := trackwin.Even(t, w.Window); err != nil && closeErr == nil {
				return err
			}
			return closeErr
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "close every pane beside the agent")
	return cmd
}

// currentTrackWindow is the tmux client and the window of the track
// $TRACKS_ID.
func currentTrackWindow(command string) (*tmux.Client, trackwin.Info, error) {
	id := os.Getenv("TRACKS_ID")
	if id == "" {
		return nil, trackwin.Info{}, errNoTrack(command)
	}
	paths, err := platform.Resolve()
	if err != nil {
		return nil, trackwin.Info{}, err
	}
	t := tmux.New(paths.TmuxSocket)
	w, err := trackWindow(t, sessionName, id)
	return t, w, err
}

// trackWindow is the window of the track with id.
func trackWindow(t trackwin.Tmux, session, id string) (trackwin.Info, error) {
	windows, err := trackwin.List(t, session)
	if err != nil {
		return trackwin.Info{}, err
	}
	for _, w := range windows {
		if w.Track == id {
			return w, nil
		}
	}
	return trackwin.Info{}, fmt.Errorf("track %s has no open window", id)
}

// paneNumber is how tracks panes shows a pane: its tmux ID without the
// %, which stays the same while other panes come and go.
func paneNumber(p tmux.Pane) string { return strings.TrimPrefix(p.ID, "%") }

// pickPanes is the panes of column that names mean, each once, in
// column's order: all of them for none.
func pickPanes(column []tmux.Pane, names []string) ([]tmux.Pane, error) {
	if len(names) == 0 {
		return column, nil
	}
	chosen := map[string]bool{}
	for _, name := range names {
		n := strings.TrimPrefix(strings.TrimSpace(name), "%")
		if _, err := strconv.Atoi(n); err != nil {
			return nil, fmt.Errorf("%q isn't a pane number; tracks panes lists them", name)
		}
		if !slices.ContainsFunc(column, func(p tmux.Pane) bool { return paneNumber(p) == n }) {
			return nil, fmt.Errorf("no pane %s beside this track's agent; tracks panes lists them", n)
		}
		chosen[n] = true
	}
	var out []tmux.Pane
	for _, p := range column {
		if chosen[paneNumber(p)] {
			out = append(out, p)
		}
	}
	return out, nil
}

// setupRunning reports whether p is a setup that hasn't finished.
func setupRunning(p tmux.Pane) bool {
	return p.Role == trackwin.RoleSetup && !strings.HasPrefix(p.State, tracks.SetupFailed)
}

// closePanes closes targets one by one, reporting each, and goes on
// past a pane that wouldn't close.
func closePanes(ctx context.Context, out io.Writer, t *tmux.Client, w trackwin.Info, targets []tmux.Pane) error {
	windows := tracks.TmuxWindows{Tmux: t, Session: sessionName}
	var failed []string
	for _, p := range ownPaneLast(targets, os.Getenv("TMUX_PANE")) {
		if p.ID == os.Getenv("TMUX_PANE") {
			// Closing it ends this command too, so say so first.
			if _, err := fmt.Fprintf(out, "Closing %s (%s), where this runs.\n", paneNumber(p), paneLabel(p)); err != nil {
				return err
			}
		}
		var err error
		switch {
		case setupRunning(p):
			err = fmt.Errorf("its setup still runs; tracks setup --wait waits for it")
		case p.Role == trackwin.RoleDevServer && p.Key != "":
			// A server removed from the repo's settings while it ran is
			// unknown to tracks down, but its pane still closes.
			if err = stopServer(ctx, w.Track, p.Key); err != nil {
				err = windows.StopPane(p)
			}
		case p.Role == trackwin.RoleReviewer:
			// Through the daemon, which ends its review and says so on
			// the relay; the pane closes whatever it says.
			if err = endReviewer(ctx, w.Track, trackwin.HandleOf(w.Track, p.Session)); err != nil {
				err = windows.EndPane(p)
			}
		default:
			err = windows.EndPane(p)
		}
		line := fmt.Sprintf("Closed %s (%s).", paneNumber(p), paneLabel(p))
		if err != nil {
			line = fmt.Sprintf("Didn't close %s (%s): %v", paneNumber(p), paneLabel(p), err)
			failed = append(failed, paneNumber(p))
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("couldn't close %s", strings.Join(failed, ", "))
	}
	return nil
}

// ownPaneLast is targets with the pane called own moved to the end.
func ownPaneLast(targets []tmux.Pane, own string) []tmux.Pane {
	i := slices.IndexFunc(targets, func(p tmux.Pane) bool { return p.ID == own })
	if i < 0 {
		return targets
	}
	return append(slices.Delete(slices.Clone(targets), i, i+1), targets[i])
}

// stopServer stops dev server key of track id through the daemon, as
// tracks down does, so the track forgets the server's error too.
func stopServer(ctx context.Context, id, key string) error {
	paths, err := platform.Resolve()
	if err != nil {
		return err
	}
	_, err = rpc.Client{Socket: paths.Socket}.Down(ctx, rpc.ServersOf(id, []string{key}))
	return err
}

// endReviewer ends track id's reviewer handle through the daemon.
func endReviewer(ctx context.Context, id, handle string) error {
	if handle == "" {
		return errors.New("the pane isn't one of this track's reviewers")
	}
	paths, err := platform.Resolve()
	if err != nil {
		return err
	}
	return rpc.Client{Socket: paths.Socket}.TrainEnd(ctx, id, handle)
}

// trackPanes are the panes beside w's agent, and its reviewers, each in
// a session of its own, last: closing one ends that reviewer too.
func trackPanes(t *tmux.Client, w trackwin.Info) ([]tmux.Pane, error) {
	column, err := trackwin.Column(t, w.Window)
	if err != nil {
		return nil, err
	}
	trains, err := trackwin.Trains(t, w.Track)
	if err != nil {
		return nil, err
	}
	for _, p := range trains {
		if p.Role == trackwin.RoleReviewer {
			column = append(column, p)
		}
	}
	return column, nil
}

// paneLabel names p's role, and its server or setup.
func paneLabel(p tmux.Pane) string {
	switch {
	case p.Role == trackwin.RoleDevServer && p.Key != "":
		return "dev server " + p.Key
	case p.Role == trackwin.RoleDevServer:
		return "dev server"
	case p.Role == trackwin.RoleReviewer && trackwin.HandleOf(p.Track, p.Session) != "":
		return "reviewer @" + trackwin.HandleOf(p.Track, p.Session)
	}
	return p.Role
}

// printPanes prints a line per pane: its number, role and what runs. A
// pane outside window, such as a reviewer, says so.
func printPanes(out io.Writer, window string, column []tmux.Pane, snap procs.Snapshot) error {
	for _, p := range column {
		label := paneLabel(p)
		if p.Window != "" && p.Window != window {
			label += ", a train"
		}
		if _, err := fmt.Fprintf(out, "%s  %s: %s\n", paneNumber(p), label, paneActivity(p, snap)); err != nil {
			return err
		}
	}
	return nil
}

// maxCommand is how much of a command line paneActivity shows.
const maxCommand = 60

// paneActivity says what runs in p: a setup's or dev server's state,
// or the programs a terminal's shell runs, and the ports they listen on.
func paneActivity(p tmux.Pane, snap procs.Snapshot) string {
	var parts []string
	switch {
	case p.Role == trackwin.RoleSetup && setupRunning(p):
		parts = append(parts, "setup running")
	case p.Role == trackwin.RoleSetup:
		parts = append(parts, "setup "+p.State)
	case p.Role == trackwin.RoleDevServer && strings.HasPrefix(p.State, tracks.ServerExited):
		parts = append(parts, "stopped ("+p.State+")")
	case p.Role == trackwin.RoleDevServer:
		parts = append(parts, "running")
	case p.Dead:
		parts = append(parts, "agent exited")
	default:
		cmds := programs(snap, p.PID)
		if len(cmds) == 0 {
			parts = append(parts, "idle shell")
		}
		for _, c := range cmds {
			if r := []rune(c); len(r) > maxCommand {
				c = string(r[:maxCommand-1]) + "…"
			}
			parts = append(parts, c)
		}
	}
	if p.Dead {
		return strings.Join(parts, "; ")
	}
	var ports []string
	for _, l := range snap.Below(p.PID) {
		if s := strconv.Itoa(l.Port); !slices.Contains(ports, s) {
			ports = append(ports, s)
		}
	}
	if len(ports) > 0 {
		parts = append(parts, "listening on "+strings.Join(ports, ", "))
	}
	return strings.Join(parts, "; ")
}

// programs are the command lines of the first processes below pid that
// aren't shells: what a terminal's shell runs.
func programs(snap procs.Snapshot, pid int) []string {
	var out []string
	visited := map[int]bool{pid: true}
	var walk func(int)
	walk = func(cur int) {
		for _, c := range snap.Children(cur) {
			if visited[c] {
				continue
			}
			visited[c] = true
			if procs.IsShell(snap.Procs[c].Command) {
				walk(c)
				continue
			}
			out = append(out, snap.Procs[c].Command)
		}
	}
	walk(pid)
	return out
}
