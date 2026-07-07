// Package newtrack drives the interactive picker flow for
// `tracks new`: template choice → repo multi-select → optional
// slug → task prompt.
//
// Uses charmbracelet/huh for the picker chrome. Keeps the daemon
// free of any terminal concerns: this package collects the outcome
// and returns it; the caller is responsible for sending it over the
// socket.
package newtrack

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tui"
	"github.com/charmbracelet/huh"
)

// ErrCancelled is returned when the user aborts the picker (Ctrl-C
// or Esc). Callers should treat this as a graceful exit, not a
// failure.
var ErrCancelled = errors.New("cancelled by user")

// Result holds the outcome of Run. Exactly one of Params (for a new
// track) or ResumeID (for resuming a finished one) is populated.
type Result struct {
	Params   daemon.NewParams
	ResumeID string
}

// Run shows the picker flow and returns the outcome. cfg must already have
// repos configured for new-track flows (the resume path does not need repos).
//
// The flow runs as two consecutive huh forms:
//
//  1. Template choice — quick select between Custom and the built-in presets,
//     including "Resume" when client is non-nil.
//  2. Repos + Slug + Task — the task field is pre-filled with the template's
//     body when one was picked, so the user can either accept as-is or tweak.
//
// When the user picks Resume, the form switches to a track-picker and returns
// Result{ResumeID: <id>}. For all other templates it returns
// Result{Params: <newParams>}.
func Run(cfg config.Config, client *daemon.Client) (Result, error) {
	template, err := pickTemplate(client != nil)
	if err != nil {
		return Result{}, err
	}

	if template == TemplateResume {
		id, err := PickResumable(client)
		if err != nil {
			return Result{}, err
		}
		return Result{ResumeID: id}, nil
	}

	if len(cfg.Repos) == 0 {
		return Result{}, errors.New("no repos configured — run `tracks` and open Settings to add some")
	}

	repoOptions := make([]huh.Option[string], 0, len(cfg.Repos))
	for _, r := range cfg.Repos {
		repoOptions = append(repoOptions, huh.NewOption(r.Name, r.Name))
	}

	if template == TemplateReview {
		params, err := runReview(repoOptions)
		return Result{Params: params}, err
	}

	// Ask/Plan are worktree-less: they run read-only against your
	// primary checkout, or against nothing at all. So repos are
	// optional — you can ask a general Ledger question unrelated to any
	// repo, or attach repos just to give Claude read context.
	worktreeless := template == TemplateAsk || template == TemplatePlan

	repoDesc := "Space to toggle, enter to confirm. Pick the repos this track should start with. Claude can request more later via the `tracks-add-repo` skill."
	repoValidate := func(v []string) error {
		if len(v) == 0 {
			return errors.New("pick at least one repo")
		}
		return nil
	}
	if worktreeless {
		repoDesc = "Optional. Space to toggle, enter to confirm. Attach repos for read-only context, or leave empty for a general question not tied to a repo."
		repoValidate = func([]string) error { return nil }
	}

	taskTitle, taskDesc := "Task prompt", "What should Claude do? Free-form. Mention a Jira-style ticket (e.g. ABC-123) and Claude will use it in the branch name and commit message."
	if template == TemplateAsk {
		taskTitle, taskDesc = "Question", "What do you want to ask? Sent to Claude as-is — no extra framing."
	}

	var (
		repos []string
		slug  string
		task  = templatePrompts[template]
	)

	build := func() *huh.Form {
		return huh.NewForm(
			huh.NewGroup(
				huh.NewMultiSelect[string]().
					Title("Repos").
					Description(repoDesc).
					Options(repoOptions...).
					Validate(repoValidate).
					Value(&repos),
				huh.NewInput().
					Title("Slug (optional)").
					Description("Short human label shown in the dashboard and used to name the track's tmux tab. Independent of the branch name (Claude picks that). Leave empty to derive a tab name from the prompt.").
					Placeholder("e.g. rate-bug-investigation").
					Value(&slug),
				huh.NewText().
					Title(taskTitle).
					Description(taskDesc).
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
	}

	if err := runFormWithDiscardConfirm(build); err != nil {
		return Result{}, err
	}

	return Result{Params: daemon.NewParams{
		Repos:      repos,
		Slug:       strings.TrimSpace(slug),
		TaskPrompt: strings.TrimSpace(task),
		Kind:       kindFor(template),
	}}, nil
}

// PickResumable shows a single-select picker over terminal-state tracks that
// have a session ID and can therefore be resumed. Returns the selected track
// ID or ErrCancelled when the user backs out.
func PickResumable(client *daemon.Client) (string, error) {
	tracks, err := client.Ls()
	if err != nil {
		return "", fmt.Errorf("daemon: %w", err)
	}

	options := []huh.Option[string]{}
	for _, t := range tracks {
		if !t.Status.IsTerminal() || t.SessionID == "" {
			continue
		}
		branch := t.Branch
		if branch == "" {
			branch = "—"
		}
		label := fmt.Sprintf("%s  [%s]  %s", shortID(t.ID), branch, trackStatus(t))
		if t.Slug != "" {
			label += "  " + t.Slug
		}
		options = append(options, huh.NewOption(label, t.ID))
	}
	if len(options) == 0 {
		return "", errors.New("no finished tracks with a session ID — nothing to resume")
	}

	var pick string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Resume which track?").
				Description("Picks up the Claude conversation from where it left off.").
				Options(options...).
				Value(&pick),
		),
	)
	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", ErrCancelled
		}
		return "", err
	}
	return pick, nil
}

// runFormWithDiscardConfirm runs the form produced by build and, when the
// user backs out (Esc / Ctrl-C), asks whether to discard their input. On
// "keep editing" it rebuilds the form — the bound Value pointers still hold
// what was typed (huh writes each field back to its pointer on every
// keystroke, and reconstructing a field re-seeds it from that pointer), so
// every field repopulates — and loops. Returns nil on a successful submit and
// ErrCancelled once the user confirms the discard.
func runFormWithDiscardConfirm(build func() *huh.Form) error {
	for {
		err := build().WithKeyMap(tui.EscQuitKeyMap()).Run()
		if err == nil {
			return nil
		}
		if !errors.Is(err, huh.ErrUserAborted) {
			return err
		}
		discard, cerr := confirmDiscard()
		if cerr != nil {
			return cerr
		}
		if discard {
			return ErrCancelled
		}
	}
}

// confirmDiscard asks whether to throw away a partially-filled new-track
// form. It returns true to discard (cancel the flow) and false to keep
// editing; "Keep editing" is the focused default so an accidental Esc can't
// silently lose work. Aborting the prompt itself (a second Esc / Ctrl-C)
// counts as a discard, keeping Esc-Esc a fast way out.
func confirmDiscard() (bool, error) {
	discard := false
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Discard this track?").
				Description("The repos, slug, and prompt you entered will be lost.").
				Affirmative("Discard").
				Negative("Keep editing").
				Value(&discard),
		),
	)
	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return true, nil
		}
		return false, err
	}
	return discard, nil
}

// runReview is the second form for the Review template. A review
// targets one repo and one PR/branch, so we use a single-select repo
// and a required target field — unlike the free-form flow, where the
// fresh placeholder branch makes the target implicit.
func runReview(repoOptions []huh.Option[string]) (daemon.NewParams, error) {
	var (
		repo      string
		reviewRef string
		slug      string
		task      = templatePrompts[TemplateReview]
	)

	build := func() *huh.Form {
		return huh.NewForm(
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Repo").
					Description("The repo the PR / branch lives in. A review targets a single repo.").
					Options(repoOptions...).
					Value(&repo),
				huh.NewInput().
					Title("PR URL or branch to review").
					Description("Paste a GitHub PR link (…/pull/123) or a branch name on origin (e.g. feat/foo). It's checked out detached so there's something to diff against base.").
					Placeholder("https://github.com/org/repo/pull/123  —or—  feat/foo").
					Validate(func(v string) error {
						if strings.TrimSpace(v) == "" {
							return errors.New("a PR URL or branch name is required for a review")
						}
						return nil
					}).
					Value(&reviewRef),
				huh.NewInput().
					Title("Slug (optional)").
					Description("Short human label shown in the dashboard and used to name the track's tmux tab. Leave empty to derive a tab name from the prompt.").
					Placeholder("e.g. rate-bug-review").
					Value(&slug),
				huh.NewText().
					Title("Task prompt").
					Description("What should Claude do? Pre-filled with the review prompt — tweak as needed.").
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
	}

	if err := runFormWithDiscardConfirm(build); err != nil {
		return daemon.NewParams{}, err
	}

	return daemon.NewParams{
		Repos:      []string{repo},
		Slug:       strings.TrimSpace(slug),
		TaskPrompt: strings.TrimSpace(task),
		ReviewRef:  strings.TrimSpace(reviewRef),
	}, nil
}

// pickTemplate is the first form: a single select between the configured
// templates. When showResume is true, a "Resume" option is appended.
// Returns ErrCancelled when the user presses Esc.
func pickTemplate(showResume bool) (Template, error) {
	choice := TemplateCustom
	options := []huh.Option[Template]{
		huh.NewOption(templateLabels[TemplateCustom], TemplateCustom),
		huh.NewOption(templateLabels[TemplateAsk], TemplateAsk),
		huh.NewOption(templateLabels[TemplatePlan], TemplatePlan),
		huh.NewOption(templateLabels[TemplateReview], TemplateReview),
	}
	if showResume {
		options = append(options, huh.NewOption(templateLabels[TemplateResume], TemplateResume))
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[Template]().
				Title("Track type").
				Description("Work edits on a branch. Ask/Plan are read-only against your primary checkout (no worktree) and can be promoted later. Review checks out a PR/branch.").
				Options(options...).
				DescriptionFunc(func() string { return templateDescriptions[choice] }, &choice).
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

func shortID(id string) string {
	if len(id) <= 15 {
		return id
	}
	return id[len(id)-15:]
}

func trackStatus(t state.Track) string {
	return string(t.Status)
}
