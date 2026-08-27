package daemon

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bluegardenproject/tracks/internal/dlog"
)

// cursorRuleTemplate is installed at ~/.cursor/rules/tracks.mdc.
//
// Two constraints shape it, both from the fact that a global rule
// loads in EVERY Cursor session the user runs, not just tracks ones:
//
//   - It must be gated. The first line tells the agent to ignore the
//     rest unless TRACKS_ID is set, so someone's ordinary Cursor work
//     is not told about worktrees it isn't in.
//   - It must be short. Its cost is paid on every unrelated session,
//     so it states what tracks is and what the agent must not assume,
//     and leaves the per-task detail to the spawn prompt.
//
// It carries no per-track values. Everything track-specific is read
// from the environment, which is why one static file suffices —
// TRACKS_ID and TRACKS_SOCKET_DIR are exported into the pane and
// inherited by whatever runs there.
//
// Verified end to end against the real CLI: inside a worktree with
// TRACKS_ID set, asking how to start a dev server answers "tracks up".
// One boundary found while checking: the rule does not load in a bare
// directory that is not a recognised workspace — an empty `git init`
// was enough to lose it. Every track pane runs in a provisioned
// worktree, so this does not bite in practice, but a repo-less ask
// track whose cwd falls back to the home directory may not get the
// rule. Its prompt carries the same contract, which is why that is a
// degradation rather than a hole.
const cursorRuleTemplate = `---
x-tracks-managed: "1"
description: How to behave inside a tracks-managed git worktree (applies only when TRACKS_ID is set)
alwaysApply: true
---

# tracks worktrees

**This rule applies only when the ` + "`TRACKS_ID`" + ` environment variable is set.**
If it is not, you are in an ordinary checkout: ignore everything below.

When it is set, you are running inside a worktree created by
[tracks](https://github.com/bluegardenproject/tracks), on a branch of its own,
in a tmux pane the user can switch into at any time.

## What that changes

- **The worktree is disposable; the user's checkout is not.** Repos attached
  read-only for reference are their PRIMARY checkouts — the working copies
  their editor watches. Never edit, commit, or push in those.
- **Dev servers belong to tracks.** Do not run ` + "`pnpm dev`" + `, ` + "`npm start`" + ` or
  similar yourself, and never background a server with ` + "`&`" + `. Run
  ` + "`tracks up`" + ` — it opens a dedicated pane and returns immediately.
  ` + "`tracks services`" + ` lists what is configured, ` + "`tracks down <name>`" + ` stops one,
  ` + "`tracks url <name>`" + ` prints the URL. ` + "`$TRACKS_ID`" + ` is already set, so
  ` + "`--track`" + ` is never needed.
- **Announce pull requests.** If you open one, put its URL on a line of its
  own as ` + "`TRACKS_PR_URL=<url>`" + ` so the dashboard picks it up. One line per PR.
- **Review before you push.** Re-read the whole diff and report what you find
  before pushing anything that isn't documentation. End with the literal line
  ` + "`REVIEW OUTCOME: pass`" + ` or ` + "`REVIEW OUTCOME: blocked`" + `, and do not push on
  blocked.
- **Stay engaged.** The session is interactive. If the task ends in a question,
  ask it and wait rather than signing off.

## Output

These sessions are read in a dashboard: keep responses terse and skip the
preamble.
`

// installCursorRule writes the global Cursor rule, if Cursor is
// installed.
//
// Gated on the binary being present so a Claude-only user does not get
// a ~/.cursor directory they never asked for. Someone who installs
// Cursor later picks the rule up on the next daemon start, which is
// frequent enough not to need its own trigger.
func (s *Server) installCursorRule(home string) error {
	binary := s.config().Cursor.Binary
	if strings.TrimSpace(binary) == "" {
		binary = "agent"
	}
	if _, err := exec.LookPath(binary); err != nil {
		dlog.Printf("skipping the Cursor rule: %s is not on PATH", binary)
		return nil
	}

	path := filepath.Join(home, ".cursor", "rules", "tracks.mdc")
	wrote, err := writeManagedFile(path, []byte(cursorRuleTemplate))
	if err != nil {
		return fmt.Errorf("write cursor rule: %w", err)
	}
	if wrote {
		dlog.Printf("installed the Cursor rule at %s", path)
	}
	return nil
}
