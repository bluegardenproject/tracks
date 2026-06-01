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

// InstallGlobalHelpers writes the tracks-add-repo skill and the
// tracks-reviewer subagent into the user's global Claude config
// (~/.claude/skills/ and ~/.claude/agents/). Claude Code's
// auto-discovery walks both global and per-project locations, so
// global install means worktrees stay clean — nothing
// `tracks`-specific ever shows up in `git status` inside a user
// repo.
//
// Called once at daemon startup. Files are overwritten on every
// call so config changes (e.g. new repos in config.yaml) refresh
// the add-repo skill's repo list.
//
// Errors are returned but treated as non-fatal by the caller: a
// missing global agent is recoverable (Claude just doesn't have
// the named subagent and the main agent has to inline the review
// itself).
func (s *Server) InstallGlobalHelpers() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}

	// Skill: tracks-add-repo (per-user content — the configured
	// repo list — embedded in the body).
	var b strings.Builder
	for _, r := range s.cfg.Repos {
		fmt.Fprintf(&b, "- `%s` — primary at `%s` (base: `%s`)\n", r.Name, r.Path, r.Base)
	}
	skillBody := fmt.Sprintf(skillTemplate, b.String())

	skillsDir := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir global skills dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "tracks-add-repo.md"), []byte(skillBody), 0o644); err != nil {
		return fmt.Errorf("write add-repo skill: %w", err)
	}

	// Subagent: tracks-reviewer (static — same for every user).
	agentsDir := filepath.Join(home, ".claude", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir global agents dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "tracks-reviewer.md"), []byte(reviewerAgentTemplate), 0o644); err != nil {
		return fmt.Errorf("write reviewer agent: %w", err)
	}

	return nil
}
