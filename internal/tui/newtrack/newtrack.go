// Package newtrack drives the interactive picker flow for
// `tracks new`: repo multi-select → branch type → slug → task prompt.
//
// Uses charmbracelet/huh for the picker chrome — same library
// stac-man uses for its config wizard. Keeps the daemon free of any
// terminal concerns: this package collects a NewParams payload and
// returns it; the caller is responsible for sending it over the
// socket.
package newtrack

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/charmbracelet/huh"
)

// ErrCancelled is returned when the user aborts the picker (Ctrl-C
// or Esc). Callers should treat this as a graceful exit, not a
// failure.
var ErrCancelled = errors.New("cancelled by user")

// slugPattern is the on-screen validation pattern. The daemon
// re-validates server-side, so this is mostly a UX nicety to fail
// fast.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Run shows the picker flow and returns the validated payload ready
// to send to the daemon. cfg must already have repos configured —
// an empty repos list is treated as a hard error since the picker
// would have nothing to show.
func Run(cfg config.Config) (daemon.NewParams, error) {
	if len(cfg.Repos) == 0 {
		return daemon.NewParams{}, errors.New("no repos configured — add some under `repos:` in ~/.config/tracks/config.yaml")
	}

	repoOptions := make([]huh.Option[string], 0, len(cfg.Repos))
	for _, r := range cfg.Repos {
		repoOptions = append(repoOptions, huh.NewOption(r.Name, r.Name))
	}

	typeOptions := make([]huh.Option[string], 0, len(cfg.Branch.Types))
	for _, t := range cfg.Branch.Types {
		typeOptions = append(typeOptions, huh.NewOption(t, t))
	}

	var (
		repos      []string
		branchType = cfg.Branch.DefaultType
		slug       string
		task       string
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Repos").
				Description("Space to toggle, enter to confirm. Pick the repos this track should start with — Claude can request more later via the `tracks-add-repo` skill.").
				Options(repoOptions...).
				Validate(func(v []string) error {
					if len(v) == 0 {
						return errors.New("pick at least one repo")
					}
					return nil
				}).
				Value(&repos),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Branch type").
				Description("Conventional-commit-style prefix. The branch will be created as <type>/<slug> in every selected worktree.").
				Options(typeOptions...).
				Value(&branchType),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Slug").
				Description("Short identifier. Lowercase, digits, and hyphens only.").
				Placeholder("e.g. swap-rate-bug").
				Validate(func(v string) error {
					v = strings.TrimSpace(v)
					if !slugPattern.MatchString(v) {
						return fmt.Errorf("must match %s", slugPattern)
					}
					return nil
				}).
				Value(&slug),
			huh.NewText().
				Title("Task prompt").
				Description("What should Claude do? Free-form. The daemon appends a TRACKS_PR_URL marker so the dashboard can pick up the PR URL reliably.").
				CharLimit(8192).
				Validate(func(v string) error {
					if strings.TrimSpace(v) == "" {
						return errors.New("task prompt is required")
					}
					return nil
				}).
				Value(&task),
		),
	)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return daemon.NewParams{}, ErrCancelled
		}
		return daemon.NewParams{}, err
	}

	return daemon.NewParams{
		Repos:      repos,
		BranchType: branchType,
		Slug:       strings.TrimSpace(slug),
		TaskPrompt: strings.TrimSpace(task),
	}, nil
}
