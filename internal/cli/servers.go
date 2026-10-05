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

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// printServers prints a line per dev server.
func printServers(out io.Writer, servers []tracks.Server) error {
	for _, sv := range servers {
		line := sv.Repo + "/" + sv.Name + ": "
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

func stateText(sv tracks.Server) string {
	line := ""
	switch sv.State {
	case tracks.ServerRunning:
		line += "running"
		if sv.Port != 0 {
			line += fmt.Sprintf(" on http://localhost:%d", sv.Port)
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
