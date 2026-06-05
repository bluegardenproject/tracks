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
	ActionNewTrack       Action = "new"
	ActionDashboard      Action = "dashboard"
	ActionList           Action = "list"
	ActionAttach         Action = "attach"
	ActionDone           Action = "done"
	ActionKill           Action = "kill"
	ActionReleaseBranch  Action = "release"
	ActionForget         Action = "forget"
	ActionPrune          Action = "prune"
	ActionSettings       Action = "settings"
	ActionGC             Action = "gc"
	ActionQuitSession    Action = "quit"
	ActionClose          Action = "close"
)

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
					huh.NewOption("Dashboard", ActionDashboard),
					huh.NewOption("Release a track's branch...", ActionReleaseBranch),
					huh.NewOption("Settings", ActionSettings),
					huh.NewOption("Garbage-collect orphan worktrees", ActionGC),
					huh.NewOption("Quit session", ActionQuitSession),
					huh.NewOption("Close menu", ActionClose),
				).
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
		label := fmt.Sprintf("%s  %s  [%s]  %s", shortID(t.ID), t.Branch, t.Status, reposLabel(t))
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

// ConfirmQuit is a small yes/no for the destructive quit action.
func ConfirmQuit(sessionName string) (bool, error) {
	return Confirm("Quit the tracks tmux session?",
		fmt.Sprintf("Kills tmux session %q and stops the daemon. Running Claude processes will be SIGTERMed.", sessionName))
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

// ActiveOnly is a PickTrack filter that excludes terminal-state
// tracks. Use for Attach / End / Kill flows.
func ActiveOnly(t state.Track) bool { return !t.Status.IsTerminal() }

// CompletedOnly is a PickTrack filter that excludes still-running
// tracks. Use for Forget / Clean flows.
func CompletedOnly(t state.Track) bool { return t.Status.IsTerminal() }

// HasLiveWorktree is a PickTrack filter that includes any track —
// regardless of status — that still owns at least one worktree
// directory on disk. Use for the Release flow: a Done track whose
// worktree is still around still locks the branch, and the user
// must be able to find it in the picker to clean it up.
func HasLiveWorktree(t state.Track) bool {
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
