package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bluegardenproject/tracks/internal/platform"
	"github.com/bluegardenproject/tracks/internal/rpc"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/tracks"
	"github.com/spf13/cobra"
)

// newTrackFlags are tracks new's flags.
type newTrackFlags struct {
	name, ref, document, prompt, promptFile, engine, model string
	repos                                                  []string
	candor                                                 int
	noOpinion, noClaimCheck                                bool
}

func newNewCmd() *cobra.Command {
	var f newTrackFlags
	cmd := &cobra.Command{
		Use:   "new <work|plan|ask|review|doc>",
		Short: "open a new track, as the New track form does",
		Long: "Opens a track of the given type in the background, without switching to it, and prints its " +
			"name. Its prompt is the new agent's task: it starts from that alone, so pass everything it needs " +
			"to know. Repos are names from the Repositories tab. A track that can't be made leaves no " +
			"draft in the New track form.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			prompt, err := readPrompt(c.InOrStdin(), f)
			if err != nil {
				return err
			}
			req, err := newRequest(args[0], f, prompt, os.Getenv("TRACKS_ID"))
			if err != nil {
				return err
			}
			paths, err := platform.Resolve()
			if err != nil {
				return err
			}
			r, err := rpc.Client{Socket: paths.Socket}.Create(c.Context(), rpc.CreateParams{Request: req},
				func(step string) { fmt.Fprintln(c.ErrOrStderr(), step) })
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(c.OutOrStdout(), "Opened %s, a %s track.\n", r.Name, req.Kind)
			return err
		},
	}
	cmd.Flags().StringVar(&f.name, "name", "", "the track's name (default: from the prompt)")
	cmd.Flags().StringSliceVar(&f.repos, "repo", nil, "a repo from the Repositories tab; repeat it for several")
	cmd.Flags().StringVar(&f.prompt, "prompt", "", "the new agent's task")
	cmd.Flags().StringVar(&f.promptFile, "prompt-file", "", "read the task from a file, - for stdin")
	cmd.Flags().StringVar(&f.ref, "ref", "", "review: the pull request link or branch")
	cmd.Flags().StringVar(&f.document, "document", "", "doc: the document to review")
	cmd.Flags().IntVar(&f.candor, "candor", 0, "review, doc: the candor level (default: Tracks' own)")
	cmd.Flags().BoolVar(&f.noOpinion, "no-opinion", false, "doc: leave out the Opinion section")
	cmd.Flags().BoolVar(&f.noClaimCheck, "no-claim-check", false, "doc: don't check the document's claims")
	cmd.Flags().StringVar(&f.engine, "engine", "", "the agent CLI (default: the one Settings picks for the type)")
	cmd.Flags().StringVar(&f.model, "model", "", "the model, with --engine (default: the engine's)")
	return cmd
}

// readPrompt is the task from --prompt or --prompt-file, "" for none.
func readPrompt(stdin io.Reader, f newTrackFlags) (string, error) {
	switch {
	case f.prompt != "" && f.promptFile != "":
		return "", errors.New("pass --prompt or --prompt-file, not both")
	case f.promptFile == "-":
		b, err := io.ReadAll(stdin)
		return string(b), err
	case f.promptFile != "":
		b, err := os.ReadFile(f.promptFile)
		return string(b), err
	}
	return f.prompt, nil
}

// newRequest is the request tracks new sends for kind, run in the track
// parent ("" outside one). The daemon checks it as it checks the form's;
// this catches what only the command line gets wrong.
func newRequest(kind string, f newTrackFlags, prompt, parent string) (tracks.Request, error) {
	k := track.Kind(strings.ToLower(strings.TrimSpace(kind)))
	if !k.Valid() {
		return tracks.Request{}, fmt.Errorf("there's no track type %q: use work, plan, ask, review or doc", kind)
	}
	if f.model != "" && f.engine == "" {
		return tracks.Request{}, errors.New("--model needs --engine")
	}
	if err := flagsFit(k, f); err != nil {
		return tracks.Request{}, err
	}
	req := tracks.Request{
		Kind: k, Name: f.name, Repos: f.repos, Prompt: strings.TrimSpace(prompt),
		ReviewRef: f.ref, Candor: f.candor, Engine: f.engine, Model: f.model, NoDraft: true,
		Parent: strings.TrimSpace(parent),
	}
	if k == track.Doc {
		req.Opinion, req.ClaimCheck = !f.noOpinion, !f.noClaimCheck
		document, err := tracks.ResolveDocument(f.document)
		if err != nil {
			return req, err
		}
		req.Document = document
	}
	return req, nil
}

// flagsFit refuses flags kind's tracks don't take, rather than drop
// them without a word.
func flagsFit(kind track.Kind, f newTrackFlags) error {
	only := []struct {
		set  bool
		flag string
		ok   bool
	}{
		{f.ref != "", "--ref", kind == track.Review},
		{f.document != "", "--document", kind == track.Doc},
		{f.noOpinion, "--no-opinion", kind == track.Doc},
		{f.noClaimCheck, "--no-claim-check", kind == track.Doc},
		{f.candor != 0, "--candor", kind == track.Review || kind == track.Doc},
	}
	for _, o := range only {
		if o.set && !o.ok {
			return fmt.Errorf("%s doesn't go with a %s track", o.flag, kind)
		}
	}
	return nil
}
