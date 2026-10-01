package claude

// addRepoSkill is v1's tracks-add-repo skill (internal/daemon/skill.go),
// renamed. v1's lists the configured repos; this one doesn't, since the
// Repositories tab changes them without the daemon hearing of it:
// `tracks add-repo` names them when it doesn't know the one asked for.
//
// The frontmatter description is what Claude sees while it looks for a
// skill, so it says when to use it.
const addRepoSkill = `---
x-tracks-managed: "1"
name: tracks-v2-add-repo
description: |
  Add another repository to the current ` + "`tracks`" + ` track. Use this when the
  task needs to touch a repo that wasn't part of the track's worktrees.
  The host CLI ` + "(`tracks`)" + ` creates a new worktree on the track's branch and
  gives you its absolute path. TRIGGER when the task references a repo by
  name and that repo's checkout is not in your current working set.
---

# tracks-v2-add-repo

You are running inside a ` + "`tracks`" + ` worktree. Your current track ID is
exported as ` + "`$TRACKS_ID`" + `.

To add another repo to the track:

` + "```bash" + `
tracks add-repo <repo-name>
` + "```" + `

The command will:

1. Create a new worktree of the repo on the track's branch.
2. Print the absolute path the worktree was checked out at.
3. Return. You can then read and write files at that path the same way
   you do in the track's other worktrees.

Only repos on the Tracks Repositories tab can be added. When the name is
not one of them, the command lists the ones that are; if the repo you
need is not among them, ask the user.
`
