// Package newtrack drives the interactive picker flow for
// `tracks new`: template choice → repo multi-select → optional
// slug → task prompt.
//
// Uses charmbracelet/huh for the picker chrome. Keeps the daemon
// free of any terminal concerns: this package collects a
// NewParams payload and returns it; the caller is responsible for
// sending it over the socket.
package newtrack

import (
	"errors"
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/tui"
	"github.com/charmbracelet/huh"
)

// ErrCancelled is returned when the user aborts the picker (Ctrl-C
// or Esc). Callers should treat this as a graceful exit, not a
// failure.
var ErrCancelled = errors.New("cancelled by user")

// Run shows the picker flow and returns the validated payload
// ready to send to the daemon. cfg must already have repos
// configured — an empty repos list is treated as a hard error.
//
// The flow runs as two consecutive huh forms:
//
//  1. Template choice — quick select between Custom and any of the
//     built-in presets.
//  2. Repos + Slug + Task — the task field is pre-filled with the
//     template's body when one was picked, so the user can either
//     accept as-is or tweak before submitting.
func Run(cfg config.Config) (daemon.NewParams, error) {
	if len(cfg.Repos) == 0 {
		return daemon.NewParams{}, errors.New("no repos configured — run `tracks` and open Settings to add some")
	}

	template, err := pickTemplate()
	if err != nil {
		return daemon.NewParams{}, err
	}

	repoOptions := make([]huh.Option[string], 0, len(cfg.Repos))
	for _, r := range cfg.Repos {
		repoOptions = append(repoOptions, huh.NewOption(r.Name, r.Name))
	}

	var (
		repos []string
		slug  string
		task  = templatePrompts[template]
	)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Repos").
				Description("Space to toggle, enter to confirm. Pick the repos this track should start with. Claude can request more later via the `tracks-add-repo` skill.").
				Options(repoOptions...).
				Validate(func(v []string) error {
					if len(v) == 0 {
						return errors.New("pick at least one repo")
					}
					return nil
				}).
				Value(&repos),
			huh.NewInput().
				Title("Slug (optional)").
				Description("Short human label shown in the dashboard. Independent of the branch name (Claude picks that). Leave empty if you don't need it.").
				Placeholder("e.g. rate-bug-investigation").
				Value(&slug),
			huh.NewText().
				Title("Task prompt").
				Description("What should Claude do? Free-form. Mention a Jira-style ticket (e.g. ABC-123) and Claude will use it in the branch name and commit message.").
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

	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return daemon.NewParams{}, ErrCancelled
		}
		return daemon.NewParams{}, err
	}

	return daemon.NewParams{
		Repos:      repos,
		Slug:       strings.TrimSpace(slug),
		TaskPrompt: strings.TrimSpace(task),
	}, nil
}

// pickTemplate is the first form: a single select between the
// configured templates. Returns TemplateCustom when the user
// presses Esc on the picker (treating "no template" the same as
// "I want a custom prompt").
func pickTemplate() (Template, error) {
	choice := TemplateCustom
	options := []huh.Option[Template]{
		huh.NewOption(templateLabels[TemplateCustom], TemplateCustom),
		huh.NewOption(templateLabels[TemplateReview], TemplateReview),
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[Template]().
				Title("Template").
				Description("Pick a starting point for the task prompt. Custom leaves it blank; Review prefills a generic code-review prompt the agent's review skills can take from there.").
				Options(options...).
				Value(&choice),
		),
	)
	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", ErrCancelled
		}
		return "", err
	}
	return choice, nil
}
