// Package cursor starts the Cursor agent CLI on a track: its chat, its
// command line and its prompts.
package cursor

import (
	"errors"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// ErrNoChat is returned for a track without a chat ID. Without
// --resume the agent would open a chat tracks has no ID for, and the
// track could never be resumed.
var ErrNoChat = errors.New("track has no chat ID")

// Command is how the Cursor agent starts on s's track, as in v1. Auto
// mode is --force, for work and review tracks only.
func Command(s agents.Spec) (agents.Start, error) {
	t := s.Track
	if len(t.Repos) == 0 && t.Kind.Worktrees() {
		return agents.Start{}, agents.ErrNoRepos
	}
	if t.Session == "" {
		return agents.Start{}, ErrNoChat
	}

	workspace := ""
	dirs := make([]string, 0, len(t.Repos))
	for i, r := range t.Repos {
		if i == 0 {
			workspace = r.Dir()
			continue
		}
		dirs = append(dirs, r.Dir())
	}
	if d := agents.DocDir(t); d != "" {
		if workspace == "" {
			workspace = d
		} else {
			dirs = append(dirs, d)
		}
	}

	prompt := strings.TrimRight(t.Prompt, " \t\n\r") + "\n\n" + agents.LinksContract
	mode, force := "", s.Auto
	switch {
	case t.Kind == track.Doc:
		// The report is written, so the track isn't read-only, but the
		// write is confirmed.
		force = false
		prompt += "\n\n" + docReviewSuffix(t)
	case t.Kind.ReadOnly():
		mode, force = string(t.Kind), false
		if len(t.Repos) > 0 {
			prompt += agents.ReadOnlySuffix
		}
	default:
		prompt += "\n\n" + taskSuffix
		if len(s.DraftPRs) > 0 {
			prompt += agents.DraftPRSuffix(s.DraftPRs, len(t.Repos))
		}
		if t.Kind == track.Review {
			prompt += reviewCandorSuffix(track.CandorLevel(t.Candor))
		}
	}

	// Cursor loads its rules only when run inside a workspace, so the
	// pane starts there, even for a doc track with repos.
	dir := workspace
	if dir == "" {
		dir = agents.Home()
	}

	line := agents.NewLine(s.Program)
	if prompt != "" && !s.Resume {
		line.Arg(prompt)
	}
	line.Set("--resume", t.Session)
	if force {
		line.Flag("--force")
	}
	line.SetIf("--mode", mode)
	line.SetIf("--workspace", workspace)
	for _, d := range dirs {
		line.Set("--add-dir", d)
	}
	if !s.Resume {
		line.SetIf("--model", t.Model)
	}
	line.SetIf("--plugin-dir", s.Hooks)
	return agents.Start{Command: s.Wrap(line.Build()), Dir: dir}, nil
}
