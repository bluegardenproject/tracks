package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// skillTemplate is the body of the Claude skill we install into each
// worktree, advertising the `tracks add-repo` capability.
//
// The skill follows Claude Code's plain-markdown convention. The
// frontmatter description is what Claude sees during skill
// discovery, so we make it self-explanatory and trigger-worthy.
const skillTemplate = `---
name: tracks-add-repo
description: |
  Mount another configured repository onto the current ` + "`tracks`" + ` track. Use
  this when the task you're solving needs to touch a repo that wasn't part
  of the initial worktree set. The host CLI ` + "(`tracks`)" + ` creates a new
  worktree under the same branch as the current track and gives you the
  absolute path. TRIGGER when the task references a repo by name and that
  repo's checkout is not visible in your current working set.
---

# tracks-add-repo

You are running inside a ` + "`tracks`" + ` worktree. Your current track ID is
exported as ` + "`$TRACKS_ID`" + `.

To add another configured repo to the track:

` + "```bash" + `
tracks add-repo <repo-name>
` + "```" + `

Configured repos available in this environment:

%s

The command will:

1. Provision a new worktree under the same branch as the current track.
2. Print the absolute path the worktree was checked out at.
3. Return — you can then read/write files at that path the same way you
   read/write files in the original worktrees.

If you need a repo that is **not** in the list above, ask the user — only
repos in the configured list can be added.
`

// installSkill writes the tracks-add-repo skill into the worktree's
// .claude/skills/ directory. The skill describes the available
// repos at install time so Claude doesn't have to discover them at
// runtime.
//
// Idempotent: an existing file is overwritten with the up-to-date
// repo list.
func (s *Server) installSkill(worktreeRoot string) error {
	// Compose the repo list bullet section. Backticks don't need
	// escaping in a regular Go string literal.
	var b strings.Builder
	for _, r := range s.cfg.Repos {
		fmt.Fprintf(&b, "- `%s` — primary at `%s` (base: `%s`)\n", r.Name, r.Path, r.Base)
	}
	body := fmt.Sprintf(skillTemplate, b.String())

	skillDir := filepath.Join(worktreeRoot, ".claude", "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("mkdir skill dir: %w", err)
	}
	target := filepath.Join(skillDir, "tracks-add-repo.md")
	return os.WriteFile(target, []byte(body), 0o644)
}
