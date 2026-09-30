# Plan: the tracks commands

**Status: planned.** Part of the [v2 masterplan](../masterplan.md), the [v2.0.0 scope](../masterplan.md#v200-scope). In a track pane `tracks` runs v2, and the prompts send agents to four commands v2 doesn't have yet: `tracks terminal`, `tracks review`, `tracks promote` and `tracks add-repo`. This plan builds them, and installs the two global helpers that point at them: the add-repo Claude skill and a Cursor rule.

## What users get

### `tracks terminal`

- Run in a track's pane, it opens a shell in the track window's right column, in the track's directory, and focuses it: "Opened a terminal in rate-bug." It's the same pane the form's Terminal checkbox opens, so several stack like the ones the Tracks window opens.
- `--track <id>` picks another track; the prompts tell agents they never need it.
- Outside a track, without `--track`: "tracks terminal works inside a track: $TRACKS_ID isn't set." A track whose window is closed: "track <id> has no open window."

### `tracks review`

Cursor's pre-push review, as v1's. Cursor can't hand work to a subagent, so this runs the reviewer as a second Cursor process that hasn't seen the conversation.

- **The diff:** the branch against `origin/<base>` of the track repo the command runs in (found from the working directory), plus the uncommitted changes. `--base <ref>` overrides the base. With no changes: "No changes against origin/main: nothing to review."
- **The reviewer:** `agent -p` with the `tracks-v2-reviewer` instructions (the Claude subagent's, frontmatter removed), the track's candor, and the diff on stdin. It runs on Cursor's default model from the Engines tab, with no permission flags, so it can read files and nothing else.
- **It prints the report,** which ends with `REVIEW OUTCOME: pass` or `REVIEW OUTCOME: blocked`, and "Running the reviewer in a separate agent session…" on stderr first.
- **Guards, as v1's:** a reviewer that runs `tracks review` gets "Already inside a review: a reviewer doesn't review itself." (an environment marker, and a lock per track in the state directory). A review times out after 15 minutes; diffs over 1 MB are cut and the reviewer is told.
- Claude tracks don't need it: their prompt calls the `tracks-v2-reviewer` subagent. The command works in them too.

### `tracks promote <id>`

An Ask or Plan track becomes a Work track with its own worktrees.

- **In Station:** open and ended Ask and Plan tracks get **Promote** (`m`) in their details, with the creation's progress in the notice line: "Promoted rate-bug: it has its own worktree now." Doc tracks can't be promoted; their follow-up is a new Work track.
- **The track stays the same one:** same ID, name, title and Station row. Its type becomes Work, its repos get worktrees on a new branch, named as Create names a Work track's, and the Work prompt's framing (edits allowed) replaces the read-only one.
- **A new session on the same agent and model** starts in a new window; the old window closes. The first message is the original task, then v1's note: "The read-only investigation/plan phase is complete. A worktree has been created on branch `<branch>`: implement the change here."
- **Claude tracks also carry the agent's last reply** after the note, so the plan isn't lost: the plan from the session's last `ExitPlanMode` when there is one, else the text of its last answer. Cut at 20 KB. Cursor tracks get v1's note only; Tracks can't read Cursor's chats.
- **Refused:** a Work, Review or Doc track ("Only Ask and Plan tracks can be promoted."), a track without repos ("rate-bug has no repos to promote."), an archived one. If making the worktrees fails, the track is left as it was.
- **The cost** counts the new session from then on. Keeping the old session's cost is a follow-up.

### `tracks add-repo <repo>`

A repo from the Repositories tab joins a running Work track.

- It gets a worktree on the track's branch, from `origin/<base>`, fetched first as Create does, and the track lists it from then on: Station, the details, Archive and Derail all include it.
- It prints the worktree's path: "Added tracks-docs at /…/worktrees/<id>/tracks-docs. You can read and write files there."
- **Refused:** a repo Tracks doesn't know, with the ones it does ("No repo named tracks-doc on the Repositories tab. Repos: tracks, tracks-docs."), one already in the track, and tracks without worktrees ("rate-bug has no worktrees: promote it first." for Ask and Plan, and Review and Doc tracks, which check out a PR or attach repos read-only).

### The global helpers

Installed when the daemon starts, next to the reviewer subagents, with the same rule: a file without Tracks' marker is the user's and stays untouched.

- **`~/.claude/skills/tracks-v2-add-repo/SKILL.md`:** v1's skill, renamed, telling Claude how to add a repo. v1's lists the repos; this one doesn't, since the Repositories tab changes them without the daemon: `tracks add-repo` names them when it doesn't know one.
- **`~/.cursor/rules/tracks-v2.mdc`:** v1's rule, which loads in every Cursor session and applies only when `TRACKS_ID` is set, without the dev-server part. It tells Cursor that this Tracks has no `tracks up` or `tracks services`, so v1's rule, where v1 installed it, doesn't send it there. Only written when Cursor is added on the Engines tab.

## How it works

- **The commands run in the pane,** where `TRACKS_ID` and `TRACKS_SOCKET_DIR` are set. `terminal` finds the track's window in tmux and adds the pane itself, as the Tracks window's key does. `review` runs git and the reviewer itself and only asks the daemon for the track's repos and candor. `promote` and `add-repo` ask the daemon, as the popups do.
- **New daemon methods:** `promote` and `add-repo`, both with progress, and `track` for one track's record.
- **Promote** reuses Create's steps: worktrees, a new session, hooks, the agent's command, the window. It saves the track's new type, repos, session and prompt in one store write, then closes the old window.
- **The last reply** comes from the session's Claude transcript, which the cost already reads.

## Order

One PR each, simplest first:

1. `tracks terminal`.
2. `tracks review`.
3. `tracks add-repo` and the Claude skill.
4. `tracks promote`, the Station action, and the Cursor rule.

## Tests

- `terminal`: the pane opens in the track's window and directory; outside a track, and with a closed window, the errors above.
- `review`: the diff's base comes from the repo the command runs in; uncommitted changes are included; an empty diff, the recursion guard and the lock; the prompt carries the instructions, candor and diff (a fake `agent`).
- `add-repo`: the worktree is on the track's branch and the store lists the repo; each refusal.
- `promote`: the track keeps its ID and name, becomes Work, and has worktrees; the prompt carries the note, and for Claude the plan or the last answer from a transcript; a failed worktree leaves the track unchanged; Doc, Work and archived tracks are refused.
- The helpers: written to the right paths, and a user's own file left alone.
