package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/spf13/cobra"
)

func newSetupCmd() *cobra.Command {
	var wait bool
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "run this track's setup, or wait for it",
		Long: "Runs the setup command of each of the track's repos that has one, such as pnpm install, " +
			"in a pane of the track's window. Without --wait it returns at once and runs failed setups again. " +
			"With --wait it starts only setups that never ran, waits until none is running, and fails when one failed. " +
			"Run inside a track: the track is $TRACKS_ID.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, id, err := trackClient("setup")
			if err != nil {
				return err
			}
			states, err := client.Setup(c.Context(), rpc.SetupParams{ID: id, Wait: wait, Retry: !wait})
			if err != nil {
				return err
			}
			return printSetup(c.OutOrStdout(), states, wait)
		},
	}
	cmd.Flags().BoolVar(&wait, "wait", false, "wait until no setup is running")
	cmd.AddCommand(newSetupDoneCmd())
	return cmd
}

// newSetupDoneCmd is what a setup pane runs when its setup succeeded.
func newSetupDoneCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:    "done",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, id, err := trackClient("setup done")
			if err != nil {
				return err
			}
			return client.SetupDone(c.Context(), rpc.SetupDoneParams{ID: id, Repo: repo})
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "the repo whose setup succeeded")
	return cmd
}

// printSetup prints a line per setup. After waiting, a failed setup
// is an error.
func printSetup(out io.Writer, states []tracks.SetupRepo, waited bool) error {
	if len(states) == 0 {
		_, err := fmt.Fprintln(out, "This track's repos have no setup.")
		return err
	}
	var failed []string
	for _, st := range states {
		line := st.Repo + ": "
		switch st.State {
		case tracks.SetupDone:
			line += "done"
		case tracks.SetupRunning:
			line += "running in its pane"
		case tracks.SetupFailed:
			if st.Code == 0 {
				line += "finished, but Tracks couldn't record it; its pane shows more"
			} else {
				line += fmt.Sprintf("failed (exit %d); its pane shows why", st.Code)
			}
			failed = append(failed, st.Repo)
		default:
			line += "not run yet"
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	if waited && len(failed) > 0 {
		return fmt.Errorf("the setup failed in %s", strings.Join(failed, ", "))
	}
	return nil
}
