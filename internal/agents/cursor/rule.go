package cursor

import (
	"path/filepath"

	"github.com/bluegardenproject/tracks/internal/agents"
)

// rule is installed at ~/.cursor/rules/tracks.mdc. A global rule loads
// in every Cursor session, so its first line gates it to track panes,
// and it stays short.
const rule = `---
x-tracks-managed: "1"
description: How to behave inside a Tracks track (applies only when TRACKS_ID is set)
alwaysApply: true
---

# Tracks tracks

**This rule applies only when the ` + "`TRACKS_ID`" + ` environment variable is set.**
If it isn't, ignore everything below.

When it is, you run in a track of
[tracks](https://github.com/bluegardenproject/tracks), in a tmux pane the
user can switch into at any time.

## What that changes

- **The worktree is disposable; the user's checkout is not.** Repos attached
  read-only for reference are their PRIMARY checkouts — the working copies
  their editor watches. Never edit, commit, or push in those.
- **There are no dev-server commands.** ` + "`tracks up`" + `, ` + "`tracks down`" + `,
  ` + "`tracks services`" + ` and ` + "`tracks url`" + ` don't exist. Never background a
  server with ` + "`&`" + `: ask the user how they run it.
- **Track commands.** ` + "`$TRACKS_ID`" + ` is already set, so ` + "`--track`" + ` is never needed.
  - ` + "`tracks terminal`" + ` opens a terminal pane beside this one, when the user
    asks for a shell.
  - ` + "`tracks review`" + ` reviews this branch in a separate agent session.
  - ` + "`tracks add-repo <repo>`" + ` checks another repo out into this track, on
    the track's branch.
  - ` + "`tracks promote`" + ` makes an Ask or Plan track a Work track with its own
    worktrees. It restarts this session, so run it only when the user asks.
- **Review comments.** When asked to work through the open comments on this
  track's PRs, follow ` + "`~/.cursor/rules/tracks-pr-comments.mdc`" + `.
- **Announce pull requests.** If you open one, put its URL on a line of its
  own as ` + "`TRACKS_PR_URL=<url>`" + ` so the dashboard picks it up. One line per PR.
- **Review before you push.** Run ` + "`tracks review`" + ` before pushing anything
  that isn't documentation, and don't push while it ends in
  ` + "`REVIEW OUTCOME: blocked`" + `.
- **Stay engaged.** The session is interactive. If the task ends in a question,
  ask it and wait rather than signing off.

## Output

These sessions are read in a dashboard: keep responses terse and skip the
preamble.
`

// prCommentsRule is how Cursor works through a track's open review
// comments. It loads only when asked for: the footer's button sends a
// prompt naming it.
const prCommentsRule = `---
x-tracks-managed: "1"
description: Work through the open review comments on a Tracks track's pull requests (only when TRACKS_ID is set)
alwaysApply: false
---

` + agents.PRCommentsSteps

// InstallRules writes the rules into home's .cursor/rules. A file there
// without the marker is the user's and stays untouched; its path is
// returned in skipped.
func InstallRules(home string) (skipped []string, err error) {
	dir := filepath.Join(home, ".cursor", "rules")
	for _, f := range []struct{ name, content string }{
		{"tracks.mdc", rule},
		{"tracks-pr-comments.mdc", prCommentsRule},
	} {
		path := filepath.Join(dir, f.name)
		ok, err := agents.WriteManaged(path, []byte(f.content))
		if err != nil {
			return skipped, err
		}
		if !ok {
			skipped = append(skipped, path)
		}
	}
	return skipped, nil
}
