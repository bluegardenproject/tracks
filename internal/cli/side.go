package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/side"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/spf13/cobra"
)

func newSideCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "side",
		Short: "start and list this track's side tracks",
		Long: "A side track is a Work track this track, its main track, starts to write code in parallel: its own " +
			"worktree, branch, window, agent, review and pull request. It joins this track's relay with a handle " +
			"of its own, reports there, and reads what this track sends it. Run inside a track: the main track " +
			"is $TRACKS_ID.",
		PersistentPreRunE: func(*cobra.Command, []string) error {
			if os.Getenv(review.EnvMarker) != "" {
				return fmt.Errorf("%w: side tracks are the track's", review.ErrAlreadyReviewing)
			}
			return nil
		},
	}
	cmd.AddCommand(newSideNewCmd(), newSideListCmd())
	return cmd
}

// sideNewFlags are tracks side new's flags.
type sideNewFlags struct {
	name, task, taskFile, engine, model string
	repos                               []string
}

func newSideNewCmd() *cobra.Command {
	var f sideNewFlags
	cmd := &cobra.Command{
		Use:   "new --task <text>",
		Short: "start a side track of this track",
		Long: "Opens a side track in the background, a Work track on this track's repos unless --repo names " +
			"others, and prints its name and its handle on the relay. The task is its prompt: its agent starts " +
			"from that alone. A side track starts no side tracks of its own.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, id, err := trackClient("side new")
			if err != nil {
				return err
			}
			task, err := readTask(c.InOrStdin(), f)
			if err != nil {
				return err
			}
			req, err := sideRequest(f, task)
			if err != nil {
				return err
			}
			req.Parent = id
			r, err := client.SideNew(c.Context(), rpc.CreateParams{Request: req},
				func(step string) { fmt.Fprintln(c.ErrOrStderr(), step) })
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(c.OutOrStdout(), side.Opened(r.Name, r.Side, r.Repos, r.KeepDrafts))
			return err
		},
	}
	cmd.Flags().StringVar(&f.task, "task", "", "the side track's task")
	cmd.Flags().StringVar(&f.taskFile, "task-file", "", "read the task from a file, - for stdin")
	cmd.Flags().StringVar(&f.name, "name", "", "the side track's name (default: from the task)")
	cmd.Flags().StringSliceVar(&f.repos, "repo", nil, "a repo from the Repositories tab; repeat it for several "+
		"(default: this track's)")
	cmd.Flags().StringVar(&f.engine, "engine", "", "the agent CLI (default: the one Settings picks for Work tracks)")
	cmd.Flags().StringVar(&f.model, "model", "", "the model, with --engine (default: the engine's)")
	return cmd
}

// readTask is the task from --task or --task-file.
func readTask(stdin io.Reader, f sideNewFlags) (string, error) {
	if f.task != "" && f.taskFile != "" {
		return "", errors.New("pass --task or --task-file, not both")
	}
	return readPrompt(stdin, newTrackFlags{prompt: f.task, promptFile: f.taskFile})
}

// sideRequest is the request tracks side new sends, its main track
// left to the caller.
func sideRequest(f sideNewFlags, task string) (tracks.Request, error) {
	switch {
	case strings.TrimSpace(task) == "":
		return tracks.Request{}, errors.New("a side track needs its task: pass --task or --task-file")
	case f.model != "" && f.engine == "":
		return tracks.Request{}, errors.New("--model needs --engine")
	}
	return tracks.Request{
		Kind: track.Work, Name: f.name, Repos: f.repos, Prompt: strings.TrimSpace(task),
		Engine: f.engine, Model: f.model, NoDraft: true, Side: true,
	}, nil
}

func newSideListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list this track's side tracks: their handle, status and pull requests",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			client, id, err := trackClient("side list")
			if err != nil {
				return err
			}
			sides, err := client.SideList(c.Context(), id)
			if err != nil {
				return err
			}
			return printSides(c.OutOrStdout(), sides)
		},
	}
}

// printSides writes side tracks as a table.
func printSides(w io.Writer, sides []rpc.SideTrack) error {
	if len(sides) == 0 {
		_, err := fmt.Fprintln(w, "No side tracks.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tHANDLE\tSTATUS\tPR")
	for _, s := range sides {
		pr := s.PR
		if pr == "" {
			pr = "-"
		}
		if len(s.PRs) > 0 {
			pr += " " + strings.Join(s.PRs, " ")
		}
		fmt.Fprintf(tw, "%s\t@%s\t%s\t%s\n", s.Name, s.Handle, s.Status, pr)
	}
	return tw.Flush()
}
