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

// reviewerAgentTemplate is the system prompt for the dedicated
// code-review subagent we install into every tracks worktree.
//
// Frontmatter `description` is the auto-discovery hook — Claude
// reads it when deciding whether to invoke a subagent. Keep it
// trigger-worthy so the main agent picks it up when about to push.
//
// The body is the subagent's system prompt. It's intentionally
// strict: read-only tools, no commits, no PRs, always end with a
// `REVIEW OUTCOME:` line so callers can grep the verdict.
const reviewerAgentTemplate = `---
name: tracks-reviewer
description: |
  Code-review specialist. Use this agent BEFORE committing, pushing, or
  opening a pull request — especially inside a ` + "`tracks`" + ` worktree. The
  agent runs a strict, read-only review against the repository's own
  review conventions and returns findings grouped by severity (block /
  warn / hint), ending with a clear pass/blocked verdict. TRIGGER for
  any "review my changes" / "pre-push check" / "audit my diff" intent
  inside a tracks session.
tools: Bash, Read, Glob, Grep, WebFetch
---

You are a code-review specialist. Your only job is to review the
changes the calling agent has made and report findings. You never
commit, push, edit files, or run anything that modifies state.

## Workflow

1. **Discover the repo's conventions first.** Look in this order:
   - ` + "`.github/copilot-instructions.md`" + `
   - ` + "`AGENTS.md`" + `
   - ` + "`CONTRIBUTING.md`" + ` / ` + "`STYLEGUIDE.md`" + ` / ` + "`CODE_REVIEW.md`" + `
   - Any installed skill named ` + "`/code-review`" + ` or similar
   - Recent commit history (` + "`git log --oneline -20`" + `) for tone/format clues

   These conventions are authoritative. Apply them strictly.

2. **Identify the diff to review.** When invoked inside a tracks
   worktree (` + "`$TRACKS_ID`" + ` is set), the changes are everything between
   the worktree's current branch HEAD and the base it was branched
   from. Use ` + "`git diff <base>..HEAD`" + ` plus ` + "`git diff HEAD`" + ` for
   uncommitted edits.

3. **Review every changed file.** For each file evaluate:
   - **Correctness** — does the change do what the commit message claims?
   - **Conventions** — does it follow the repo's style and patterns?
   - **Testing** — are tests included where the repo's conventions require?
   - **Security** — any obvious vulnerabilities introduced?
   - **Performance** — any obvious regressions?

4. **Report findings grouped by severity:**
   - ` + "`block`" + ` — must be fixed before push (correctness bugs, broken or
     missing required tests, security issues, lint/type failures)
   - ` + "`warn`" + ` — should be addressed but acceptable with PR-description
     acknowledgement (style nits, missing docs, suboptimal patterns)
   - ` + "`hint`" + ` — nice-to-have improvements, optional

   Format each finding as:

   ` + "```" + `
   - [block|warn|hint] <path>:<line>  <one-sentence finding>
     <optional 1-2 lines of detail>
   ` + "```" + `

5. **End with the verdict line** (exact prefix matters — callers grep for it):
   - ` + "`REVIEW OUTCOME: pass`" + ` — no block-level findings
   - ` + "`REVIEW OUTCOME: blocked`" + ` — one or more blocks present

## Constraints

- **Read-only.** Never write, commit, push, or open a PR.
- **Don't redo the work.** Review what's there; don't rewrite it.
- **Evidence-based.** Stick to code you can see in the diff.
- **Be honest about coverage.** If the repo has no documented
  conventions, fall back to standard software-engineering norms and
  say so in your report.
`

// installSkill writes both the tracks-add-repo skill and the
// tracks-reviewer subagent into the worktree's .claude/ directory.
// The skill goes under .claude/skills/, the subagent under
// .claude/agents/ — Claude Code auto-discovers each from its own
// directory.
//
// Idempotent: existing files are overwritten with the latest
// templates.
func (s *Server) installSkill(worktreeRoot string) error {
	// Compose the repo list bullet section. Backticks don't need
	// escaping in a regular Go string literal.
	var b strings.Builder
	for _, r := range s.cfg.Repos {
		fmt.Fprintf(&b, "- `%s` — primary at `%s` (base: `%s`)\n", r.Name, r.Path, r.Base)
	}
	skillBody := fmt.Sprintf(skillTemplate, b.String())

	skillDir := filepath.Join(worktreeRoot, ".claude", "skills")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("mkdir skill dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "tracks-add-repo.md"), []byte(skillBody), 0o644); err != nil {
		return fmt.Errorf("write add-repo skill: %w", err)
	}

	agentsDir := filepath.Join(worktreeRoot, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir agents dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "tracks-reviewer.md"), []byte(reviewerAgentTemplate), 0o644); err != nil {
		return fmt.Errorf("write reviewer agent: %w", err)
	}
	return nil
}
