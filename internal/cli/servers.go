package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/spf13/cobra"
)

// trackClient is the daemon's client and the track a command inside a
// track is about.
func trackClient(command string) (rpc.Client, string, error) {
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

func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up [server]",
		Short: "start this track's dev servers, or one of them",
		Long: "Starts each dev server of the track's repos, from the Repositories tab, in a pane of the track's " +
			"window, once its repo's setup is done. Name a server, or repo/server, to start only it. " +
			"Run inside a track: the track is $TRACKS_ID.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			client, id, err := trackClient("up")
			if err != nil {
				return err
			}
			servers, err := client.Up(c.Context(), rpc.ServersParams{ID: id, Server: firstArg(args)})
			if err != nil {
				return err
			}
			if err := printServers(c.OutOrStdout(), servers); err != nil {
				return err
			}
			if slices.ContainsFunc(servers, func(sv tracks.Server) bool { return sv.Problem != "" }) {
				return errors.New("some servers didn't start")
			}
			return nil
		},
	}
}

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down [server]",
		Short: "stop this track's dev servers, or one of them",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			client, id, err := trackClient("down")
			if err != nil {
				return err
			}
			servers, err := client.Down(c.Context(), rpc.ServersParams{ID: id, Server: firstArg(args)})
			if err != nil {
				return err
			}
			return printServers(c.OutOrStdout(), servers)
		},
	}
}

func newLogsCmd() *cobra.Command {
	var lines int
	cmd := &cobra.Command{
		Use:   "logs <server>",
		Short: "print what one of this track's dev servers printed last",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			client, id, err := trackClient("logs")
			if err != nil {
				return err
			}
			text, err := client.Logs(c.Context(), rpc.LogsParams{ID: id, Server: args[0], Lines: lines})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), strings.TrimRight(text, "\n"))
			return err
		},
	}
	cmd.Flags().IntVarP(&lines, "lines", "n", 200, "how many lines")
	return cmd
}

func newServersCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "servers",
		Short: "list this track's dev servers and what else listens in its panes",
		Long: "Lists the track's dev servers with their state and port, then the servers something else in " +
			"the track's panes started, such as the agent. With --all, or outside a track, every open track's.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			id := ""
			if !all {
				id = os.Getenv("TRACKS_ID")
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			servers, err := rpc.Client{Socket: paths.Socket}.Servers(c.Context(), id)
			if err != nil {
				return err
			}
			if len(servers) == 0 {
				_, err := fmt.Fprintln(c.OutOrStdout(), "No dev servers.")
				return err
			}
			if id == "" {
				return printAllServers(c.OutOrStdout(), servers)
			}
			return printServers(c.OutOrStdout(), servers)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "every open track's servers")
	return cmd
}

func newURLCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "url <server>",
		Short: "print the address of one of this track's dev servers",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			client, id, err := trackClient("url")
			if err != nil {
				return err
			}
			servers, err := client.Servers(c.Context(), id)
			if err != nil {
				return err
			}
			url, err := serverURL(servers, args[0])
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), url)
			return err
		},
	}
}

// serverURL is the address of the server called repo/name, or name
// when only one repo has a server called that, as tracks up matches.
func serverURL(servers []tracks.Server, name string) (string, error) {
	var exact, named []tracks.Server
	for _, sv := range servers {
		switch {
		case sv.Name == "":
		case strings.EqualFold(sv.Repo+"/"+sv.Name, name):
			exact = append(exact, sv)
		case strings.EqualFold(sv.Name, name):
			named = append(named, sv)
		}
	}
	found := exact
	if len(found) == 0 {
		found = named
	}
	switch {
	case len(found) == 0:
		return "", fmt.Errorf("no dev server %s in this track", name)
	case len(found) > 1:
		return "", fmt.Errorf("more than one repo has a server %s; name it as repo/server", name)
	case found[0].Port == 0:
		return "", fmt.Errorf("%s isn't listening on a port yet", serverLabel(found[0]))
	}
	return fmt.Sprintf("http://localhost:%d", found[0].Port), nil
}

// printAllServers prints every track's servers under its name.
func printAllServers(out io.Writer, servers []tracks.Server) error {
	for i, sv := range servers {
		if i == 0 || servers[i-1].Track != sv.Track {
			if _, err := fmt.Fprintln(out, sv.TrackName); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out, "  "+serverLabel(sv)+": "+stateText(sv)); err != nil {
			return err
		}
	}
	return nil
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// printServers prints a line per dev server.
func printServers(out io.Writer, servers []tracks.Server) error {
	for _, sv := range servers {
		line := serverLabel(sv) + ": "
		switch {
		case sv.Problem != "":
			line += "not started: " + sv.Problem
		default:
			line += stateText(sv)
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

// serverLabel names sv: repo/name, or for a server Tracks didn't start,
// its port and type.
func serverLabel(sv tracks.Server) string {
	if sv.Name != "" {
		return sv.Repo + "/" + sv.Name
	}
	return fmt.Sprintf(":%d (%s, not started by Tracks)", sv.Port, sv.Type)
}

func stateText(sv tracks.Server) string {
	line := ""
	switch sv.State {
	case tracks.ServerReady:
		line += fmt.Sprintf("ready on http://localhost:%d", sv.Port)
	case tracks.ServerStarting:
		line += "starting"
		if sv.Port != 0 {
			line += fmt.Sprintf(", for http://localhost:%d", sv.Port)
		}
	case tracks.ServerExited:
		line += "stopped by itself; its pane shows its output"
	case tracks.ServerCrashed:
		line += fmt.Sprintf("crashed (exit %d); its pane shows why", sv.Code)
	default:
		line += "stopped"
	}
	return line
}
