// Package menu renders the `tracks` overlay menu — the popup the
// user opens with `<prefix>+<key>` from any tmux window.
//
// The menu is intentionally tiny: a single huh.Select that returns
// an Action. The caller (cmd/menu.go) dispatches the action.
// Sub-pickers (e.g. "attach to which track?") also live here so
// the popup stays a single subprocess and the user never has to
// navigate between processes.
package menu

import (
	"errors"
	"fmt"
	"os"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/daemon"
	"github.com/bluegardenproject/tracks/internal/state"
	"github.com/bluegardenproject/tracks/internal/tui"
	"github.com/charmbracelet/huh"
)

// ErrCancelled signals the user closed the menu without picking
// anything (Esc / Ctrl-C). Callers should treat this as success
// with no action.
var ErrCancelled = errors.New("cancelled")

// ErrNoTracks is returned by PickTrack when the filter rules out
// every track. Callers should print a helpful message and pause
// rather than just closing the popup.
var ErrNoTracks = errors.New("no tracks match the filter")

// Action identifies one top-level menu choice.
type Action string

const (
	ActionNewTrack      Action = "new"
	ActionResumeTrack   Action = "resume"
	ActionReopen        Action = "reopen"
	ActionDashboard     Action = "dashboard"
	ActionList          Action = "list"
	ActionAttach        Action = "attach"
	ActionDone          Action = "done"
	ActionKill          Action = "kill"
	ActionAddRepo       Action = "add_repo"
	ActionPromote       Action = "promote"
	ActionReleaseBranch Action = "release"
	ActionForget        Action = "forget"
	ActionPrune         Action = "prune"
	ActionProxy         Action = "proxy"
	ActionSettings      Action = "settings"
	ActionGC            Action = "gc"
	ActionQuitSession   Action = "quit"
	ActionClose         Action = "close"
)

// actionHints give the menu a one-line description under the focused
// option so capabilities like add-repo / promote are discoverable.
var actionHints = map[Action]string{
	ActionNewTrack:      "Pick a type (work / ask / plan / review), then create it.",
	ActionResumeTrack:   "Continue a finished track's Claude conversation from where it left off.",
	ActionReopen:        "Bring back every track that was still running when tracks was last quit.",
	ActionDashboard:     "Live list of all tracks, their status and PRs.",
	ActionAddRepo:       "Realised the change spans another repo? Mount it onto a running track.",
	ActionPromote:       "Done investigating? Turn a read-only ask/plan track into a worktree to implement.",
	ActionReleaseBranch: "Remove a finished track's worktree (keeps the branch locally).",
	ActionProxy:         "Show stable-port proxy status (fixed port → active track's service).",
	ActionSettings:      "Add, edit, or remove repos.",
	ActionGC:            "Clean up orphaned worktree directories.",
	ActionQuitSession:   "Kill the tmux session and stop the daemon.",
	ActionClose:         "Close this menu.",
}

// PickAction shows the top-level menu and returns the user's choice.
func PickAction() (Action, error) {
	var pick Action
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[Action]().
				Title("tracks").
				Description("Up/Down to navigate, Enter to select, Esc to close.").
				Options(
					huh.NewOption("New track", ActionNewTrack),
					huh.NewOption("Resume a track…", ActionResumeTrack),
					huh.NewOption("Reopen interrupted tracks", ActionReopen),
					huh.NewOption("Dashboard", ActionDashboard),
					huh.NewOption("Add repo to a track…", ActionAddRepo),
					huh.NewOption("Promote a read-only track…", ActionPromote),
					huh.NewOption("Release a track's branch...", ActionReleaseBranch),
					huh.NewOption("Proxy status", ActionProxy),
					huh.NewOption("Settings", ActionSettings),
					huh.NewOption("Garbage-collect orphan worktrees", ActionGC),
					huh.NewOption("Quit session", ActionQuitSession),
					huh.NewOption("Close menu", ActionClose),
				).
				DescriptionFunc(func() string { return actionHints[pick] }, &pick).
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

// PickTrack shows a single-select picker over the live track list.
// filter, when non-nil, is applied to each track; only tracks where
// filter returns true are offered. Pass nil to show all tracks.
func PickTrack(client *daemon.Client, title string, filter func(state.Track) bool) (state.Track, error) {
	tracks, err := client.Ls()
	if err != nil {
		return state.Track{}, fmt.Errorf("daemon: %w", err)
	}
	options := []huh.Option[string]{}
	byID := map[string]state.Track{}
	for _, t := range tracks {
		if filter != nil && !filter(t) {
			continue
		}
		branch := t.Branch
		if branch == "" {
			branch = "—"
		}
		label := fmt.Sprintf("%s  [%s]  %s  [%s]  %s", shortID(t.ID), kindOf(t), branch, t.StatusLabel(), reposLabel(t))
		options = append(options, huh.NewOption(label, t.ID))
		byID[t.ID] = t
	}
	if len(options) == 0 {
		return state.Track{}, ErrNoTracks
	}
	var pick string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Description("Up/Down to navigate, Enter to select, Esc to cancel.").
				Options(options...).
				Value(&pick),
		),
	)
	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return state.Track{}, ErrCancelled
		}
		return state.Track{}, err
	}
	return byID[pick], nil
}

// ErrNoRepos is returned by PickConfigRepo when every configured repo
// is excluded (already in the track).
var ErrNoRepos = errors.New("no repos available to add")

// PickConfigRepo shows a single-select over configured repos, skipping
// any whose name is in exclude. Use for the Add-repo flow.
func PickConfigRepo(cfg config.Config, exclude map[string]bool, title string) (string, error) {
	options := []huh.Option[string]{}
	for _, r := range cfg.Repos {
		if exclude[r.Name] {
			continue
		}
		options = append(options, huh.NewOption(fmt.Sprintf("%s  (base: %s)", r.Name, r.Base), r.Name))
	}
	if len(options) == 0 {
		return "", ErrNoRepos
	}
	var pick string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Description("Up/Down to navigate, Enter to select, Esc to cancel.").
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

// ConfirmQuit is a small yes/no for the destructive quit action.
func ConfirmQuit(sessionName string) (bool, error) {
	return Confirm("Quit the tracks tmux session?",
		fmt.Sprintf("Kills tmux session %q and stops the daemon. Running Claude processes will be SIGTERMed.", sessionName))
}

// ConfirmRemoveTrack is the yes/no gate in front of removing a track for
// good. Removal drops the record — task prompt, cost, PR links — so the
// prompt names the track and spells out what survives.
func ConfirmRemoveTrack(t state.Track) (bool, error) {
	what := shortID(t.ID)
	if t.Slug != "" {
		what += " (" + t.Slug + ")"
	}
	body := "Drops its record from tracks — task prompt, cost and PR links go " +
		"with it. Any branch it created stays in the repo. This cannot be undone."
	if t.Status == state.StatusInterrupted {
		// The record is the only handle `tracks reopen` has, and an
		// interrupted track's worktree is still on disk.
		body += " This track was interrupted, not closed: removing it gives up " +
			"reopening it, and leaves its worktree for the next `tracks gc`."
	}
	return Confirm(fmt.Sprintf("Remove track %s?", what), body)
}

// Confirm is a generic yes/no popup used by any menu action that
// needs an explicit go-ahead before doing something destructive.
func Confirm(title, description string) (bool, error) {
	var yes bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(description).
				Affirmative("Yes").
				Negative("Cancel").
				Value(&yes),
		),
	)
	if err := form.WithKeyMap(tui.EscQuitKeyMap()).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return false, ErrCancelled
		}
		return false, err
	}
	return yes, nil
}

func shortID(id string) string {
	if len(id) <= 15 {
		return id
	}
	return id[len(id)-15:]
}

func reposLabel(t state.Track) string {
	names := make([]string, 0, len(t.Repos))
	for _, r := range t.Repos {
		names = append(names, r.Name)
	}
	return joinShort(names, 32)
}

func joinShort(items []string, max int) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ","
		}
		out += s
		if len(out) > max {
			return out[:max-1] + "…"
		}
	}
	return out
}

// kindOf returns a track's kind for display, defaulting to work for
// pre-migration entries with an empty kind.
func kindOf(t state.Track) state.Kind {
	if t.Kind == "" {
		return state.KindWork
	}
	return t.Kind
}

// ActiveOnly is a PickTrack filter that excludes terminal-state tracks
// and drafts. Use for Attach / Close / Kill flows — a draft has no window,
// process, or worktree to act on (it's launched with L / dismissed with x).
func ActiveOnly(t state.Track) bool {
	return !t.Status.IsTerminal() && t.Status != state.StatusDraft
}

// PromotableOnly filters to active worktree-less (ask/plan) tracks —
// the only ones that can be promoted. Doc reviews are worktree-less too
// but have no code work to promote *to* (see handlePromote), so they
// stay out of the picker.
func PromotableOnly(t state.Track) bool {
	return !t.Status.IsTerminal() && t.Status != state.StatusDraft &&
		t.Kind.Worktreeless() && t.Kind != state.KindDoc
}

// WorktreeTrack filters to active tracks that own worktrees (work /
// review) — the only ones a repo can be added to. Drafts own no worktree
// yet, so they're excluded.
func WorktreeTrack(t state.Track) bool {
	return !t.Status.IsTerminal() && t.Status != state.StatusDraft && !t.Kind.Worktreeless()
}

// FinishedOnly is a PickTrack filter that excludes still-running
// tracks. Use for Remove / Clear flows. Interrupted tracks are offered
// too — removing one is an explicit choice to stop intending to
// reopen it, which is exactly what this picker is for.
func FinishedOnly(t state.Track) bool { return t.Status.IsTerminal() }

// HasLiveWorktree is a PickTrack filter that includes any track —
// regardless of status — that still owns at least one worktree
// directory on disk. Use for the Release flow: a Done track whose
// worktree is still around still locks the branch, and the user
// must be able to find it in the picker to clean it up.
func HasLiveWorktree(t state.Track) bool {
	// Worktree-less tracks hold the primary checkout paths (which always
	// exist) — they own no tracks worktree to release.
	if t.Kind.Worktreeless() {
		return false
	}
	for _, r := range t.Repos {
		if r.Path == "" {
			continue
		}
		if _, err := os.Stat(r.Path); err == nil {
			return true
		}
	}
	return false
}
