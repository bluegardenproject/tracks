// Package claude starts Claude Code on a track: its command line, its
// prompts and the reviewer subagents those prompts call.
package claude

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

// ErrNoSession is returned when resuming a track without a session ID.
var ErrNoSession = errors.New("track has no session ID")

// Command is how Claude Code starts on s's track, as in v1. Unlike v1,
// an ask or plan track resumes in plan mode too.
func Command(s agents.Spec) (agents.Start, error) {
	t := s.Track
	if len(t.Repos) == 0 && t.Kind.Worktrees() {
		return agents.Start{}, agents.ErrNoRepos
	}
	dirs := make([]string, 0, len(t.Repos)+1)
	for _, r := range t.Repos {
		dirs = append(dirs, r.Dir())
	}
	docDir := agents.DocDir(t)
	if docDir != "" {
		dirs = append(dirs, docDir)
	}

	prompt := strings.TrimRight(t.Prompt, " \t\n\r")
	mode := "default"
	if s.Auto {
		mode = "auto"
	}
	switch {
	case t.Kind == track.Doc:
		mode = docPermissionMode(mode)
		prompt += "\n\n" + fmt.Sprintf(docReviewTemplate, t.Document, docReviewBrief(t))
	case t.Kind.ReadOnly():
		mode = "plan"
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

	dir := docDir
	switch {
	case dir != "":
	case len(t.Repos) > 0:
		dir = t.Repos[0].Dir()
	default:
		dir = agents.Home()
	}

	line := agents.NewLine(s.Program)
	if s.Resume {
		if t.Session == "" {
			return agents.Start{}, ErrNoSession
		}
		line.Set("--resume", t.Session)
	} else {
		if prompt != "" {
			line.Arg(prompt)
		}
		line.SetIf("--session-id", t.Session)
	}
	line.SetIf("--permission-mode", mode)
	if !s.Resume {
		line.SetIf("--model", t.Model)
	}
	for _, d := range dirs {
		line.Set("--add-dir", d)
	}
	line.SetIf("--settings", s.Hooks)
	return agents.Start{Command: s.Wrap(line.Build()), Dir: dir}, nil
}

// NewSession is a fresh session ID: Claude takes one with --session-id,
// so the transcript can be found later.
func NewSession() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
