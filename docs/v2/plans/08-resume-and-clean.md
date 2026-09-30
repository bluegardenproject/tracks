# Plan: resuming and cleaning tracks

**Status: built.** Part of the [v2 masterplan](../masterplan.md), chunk 5 (real tracks). Ended tracks stay in Station. **Resume** starts their agent again on its session, and **Clean** removes their worktrees for good. Clean was later replaced by Archive, which removes the branches too: see [13-archive-replaces-clean.md](13-archive-replaces-clean.md).

## What users get

- **Ended tracks stay in Station's list** with the open ones: the open ones first, in window order, then the ended ones, most recently ended first. An ended track has no window number, and its status is **ended**, or **cleaned** once Clean removed its worktrees. Filters come later.
- **An ended track's details** offer **Resume** (`r`), **Clean** (`l`), Copy path, Copy session and Open PR. Enter resumes it. A cleaned track can't be resumed or cleaned again; a way to resume one comes later, through the settings.
- **Resume:**
  - The track's window opens again under its name, or with `-2` when the name is taken, with a terminal pane if the track had one. Station switches to it, as Open does.
  - The engine continues the session, as in v1: `claude --resume <session>` or `agent --resume <chat>`, without the prompt and without `--model`, since a resumed session keeps its model. The permission mode and folders are Create's. Unlike v1, Ask and Plan resume in plan mode; v1 resumes them in the configured mode.
  - An ended track keeps its worktrees, so a missing one is a bug, not something to fix quietly. Resume stops and asks: "Worktree couldn't be found", with the repo and path, and **Re-create worktree** / **Cancel**. Only `y` or a click re-creates it; Enter cancels.
  - Re-creating brings a Work worktree back from its branch, and a Review's by fetching its PR or branch again. A branch that's gone is an error: "tracks/abc123 no longer exists in web."
  - The track's engine must still be on the Engines tab: "Add Cursor on the Engines tab to resume this track."
  - Progress shows in Station's hint row ("Re-creating the worktree for web…", "Starting Claude Code…"). A failure undoes what Resume made, the window and the re-created worktrees, and says why; the track stays ended.
- **Clean** removes an ended track's worktrees and their folder. Branches stay, as in v1. Tracks without worktrees (Ask, Plan, Doc) have no Clean.
  - It checks every worktree first. When one has changed or untracked files or commits that exist nowhere else, it lists them and asks: "web: 3 changed files and 1 commit that exists nowhere else. Remove anyway / Cancel". Only `y` or a click removes then; Enter cancels, since on an ended track it otherwise resumes. Otherwise it asks once: "Remove the worktrees of rate-bug? Its branches stay."
  - Before removing, it records the branch each Work worktree is on, since the agent renames `tracks/abc123`.
  - Commits count as existing nowhere else when no remote branch has them. A Review worktree is detached, so for it these are the commits made since its checkout.
  - A cleaned track shows "removed" in place of its worktree paths.
- **End** stays as it is: the window closes, and the worktrees stay.

## How a track is resumed

1. **Claim:** one Resume, Clean or End at a time per track; another one meanwhile gets "rate-bug is busy." The daemon is the only writer, so the claim is kept in memory.
2. **Check:** the track is ended and not cleaned, has a session, and its engine is added. When a worktree is gone, Resume returns the missing ones and does nothing else, unless asked to re-create them.
3. **Name:** its name if no window has it, else the next free `-2`, `-3`, as Create does.
4. **Worktrees,** when asked to: each missing one is re-created, after `git worktree prune` in its primary checkout:
   - **Work:** `git worktree add <path> <branch>`.
   - **Review:** fetch the PR's `pull/<n>/head` or the branch, then check it out detached, as Create does.
5. **Command:** the engine's resume command line.
6. **Window:** opened with `@tracks_id`, as Create does.
7. **Save:** `closed_at` and `cleaned_at` are cleared last, so the sweep never closes a track while it's being resumed.

A failure undoes the steps before it: the window closes, and only the worktrees Resume re-created are removed.

## Watching

- **Healing:** the sweep also does the opposite of closing. A track recorded as ended whose window exists, for example after the daemon died between opening a window and saving, is recorded as open again.

## The daemon

- **Methods:** `resume` and `clean` join `create`, `list` and `end`. `resume` answers with the worktrees it couldn't find, and with `recreate` re-creates them. `clean` with `check` only looks, for Station's first question. Without it, `clean` removes the worktrees when it finds no unsaved work, and otherwise returns what it found and removes nothing. With `force`, which "Remove anyway" sends, it removes them anyway.
- **`list`** returns the ended tracks too.

## Database

`store/migrations/0003_cleaned.sql`:

```sql
ALTER TABLE tracks ADD COLUMN cleaned_at INTEGER; -- its worktrees were removed
```

- Clean sets it. Station reads it, instead of checking every worktree on disk every 2 s.
- **Station's list:** the open tracks plus the 100 most recently ended ones. Both are served by the existing `tracks_open (closed_at, created_at)` index. Paging comes with History.

## Packages

No new packages. Changed:
- `track`: when a track closed and was cleaned.
- `store`: the migration, the list with ended tracks, and recording a resume and a clean.
- `workspace`: re-creating missing worktrees, checking for unsaved work (v1's `UnsavedWork`, plus the Review case), and removing the worktrees while keeping the branches.
- `agents/claude` and `agents/cursor`: the resume command lines.
- `tracks`: Resume, Clean, the per-track claim, ended tracks in List, and the sweep's healing.
- `rpc` and `daemon`: the `resume` and `clean` methods.
- `ui/source`: ended tracks, their status, and removed worktrees.
- `ui/tracksview`: ended rows, their actions, Clean's and Resume's questions, and Resume's progress.
- `cli`: Station's Resume and Clean through the daemon.

## Not in this plan

- Resuming every track at once after a restart, as v1's reopen does.
- Resuming a cleaned track: later, behind a setting.
- Cleaning or archiving automatically, removing a track from the list, and filters: with History (chunk 4).
- Promote.
- Statuses beyond open and ended: with the status model (chunk 6).

## Tests

- **Command lines:** `agents/claude` and `agents/cursor` build the same resume command lines as v1's `BuildResumeOptions` and `ShellCommand`, except for Ask and Plan's mode.
- **`workspace`,** on a temp repo with a bare origin: re-creating a Work worktree from its branch and a Review one from its ref; each kind of unsaved work, and a Review worktree with and without commits of its own; removal keeping the branch.
- **`store`:** the migration, the list's order and limit, and recording a resume and a clean.
- **`tracks`,** with fakes: Resume's steps and rollback when each step fails, a missing worktree, a busy or cleaned track, Clean refusing and then forced, and the sweep's healing.
- **`ui/tracksview`:** ended and cleaned rows and their actions, Clean's "Remove anyway", and Resume's "Re-create worktree".
- **End to end, isolated:** create a Work track, end it and resume it, delete its worktree and resume it again, then clean it, with a fake `claude` on `PATH` that prints its arguments.

## Delivery

One branch, `feat/v2-resume-clean`, with these commits: this plan; `track` and `store`; `workspace`; the command lines in `agents`; `tracks`; `rpc` and `daemon`; Clean's check alone; Station; docs; keeping renamed branches; no Resume after Clean, and asking about a missing worktree.
