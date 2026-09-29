# Plan: change stream and Derail

**Status: change stream built, Derail planned.** Part of the [v2 masterplan](../masterplan.md). It finishes chunk 4 (storage): the daemon tells the Tracks window when tracks change, instead of Station reading the list every 2 seconds. It also adds **Derail**, which deletes a track for good. The event timeline and the rest of History (text search, more filters, paging) move to the masterplan's [Future features](../masterplan.md#future-features).

## What users get

- **Station updates at once** when anything changes: a track is created, ends, waits on a dialog, gets a PR, is archived, or its window is renamed or moved. Nothing reads the list on a timer any more.
- **Derail** (`d`) in an ended or archived track's details deletes the track: its worktrees, its local branches and its record. It can't be undone.

## Change stream

- **`watch`**, a new daemon method, streams one line per change and never returns on its own. The first line comes at once, so a client that (re)connects reads the list once. Changes that come close together are sent as one.
- **What counts as a change:**
  - every write to the tracks in the database: creating, a status event, a rename, a branch name, a PR found or changed, archive and unarchive, the filter, Derail;
  - the track windows as tmux lists them (which windows, their order and numbers), compared on each daemon tick (the existing 2-second Sweep), since tmux doesn't tell the daemon when a window moves.
- **How it's built:** a `Changes` broadcaster in `tracks` (subscribe, notify, close); the Store the daemon gives `tracks` is wrapped so that each successful write notifies; Sweep notifies when the window list differs from its last one. At shutdown the daemon closes `Changes` first, which ends every `watch`, so it doesn't wait on them.
- **Station:** the Tracks window runs one `watch` and reads the list on every line. The 2-second refresh is dropped. When the stream breaks (a daemon restart, a new build), it tries again every second and reads the list once connected. The hint row shows "Reconnecting to the daemon…" while it's down.
- **Unchanged:** the daemon's 2-second tick (Sweep and the pane check), the 1-minute GitHub poll, and the footer's 15 seconds.

## Derail

- **Only ended tracks,** done or closed, archived ones included. An open track shows End; Derail appears once it's ended.
- **It deletes:** the track's worktrees (if Clean hasn't removed them), its local branches in the repos' checkouts, its hooks folder, and its row in the database with its repos and PRs. Pushed branches and PRs on GitHub stay. The agent's own session files stay where the agent keeps them.
- **Before deleting,** it checks for work that would be lost: unsaved work in the worktrees, as Clean does, and commits that exist only on the track's branches (on no other branch and no remote), which matters once the worktrees are gone. It lists what it finds and offers **Derail anyway** or Cancel. Without any, it asks: "Derail rate-bug? Its worktrees, branches and record are deleted for good." with **Derail** or Cancel. Enter never confirms; `y` does.
- **An open PR** adds a line to the question: "Its PR #12 stays open on GitHub."
- **Key:** `d`.

## Model

- `tracks.Changes` with `Subscribe() (<-chan struct{}, func())`, `Notify()` and `Close()`; channels of one, so a slow reader gets one line for many changes.
- `tracks.Service`: `Derail(ctx, id, force) ([]workspace.Unsaved, error)`; Sweep compares the window list.
- `workspace`: the commits that exist only on a track's branches; deleting its branches.
- `store`: `DeleteTrack(id)`, with its repos and PRs.
- `rpc`, `daemon`: `watch` and `derail`; the wrapped Store; closing `Changes` at shutdown.
- `ui/tracksview`: the stream instead of the refresh; the Derail button and its questions. `cli`: the wiring.

## Tests

- **`tracks`:** Changes coalescing and close; a notification for each kind of write and for a moved window, none for a Sweep that changes nothing; Derail on an open track (refused), with unsaved work, with commits only on the branch, on a cleaned and an archived track, and what it deletes.
- **`workspace`:** branch-only commits in a real git repo (pushed, merged into another branch, and not); deleting branches.
- **`store`:** DeleteTrack takes the repos and PRs with it.
- **`daemon`:** `watch` sends a first line, a line after a change, and ends at shutdown; `derail` on a missing track.
- **`ui`:** Station reloads on each line and reconnects after the stream breaks; the Derail button and its questions.
- **By hand:** a track created, ended and archived shows up in Station at once; Derail a track and check its worktree, branch and record are gone.

## Delivery

Two branches, each its own PR:
1. **`feat/v2-change-stream`:** this plan and the masterplan's Future features; the change stream and Station on it.
2. **`feat/v2-derail`:** Derail; chunk 4 marked done in the masterplan, and plans 11 and 12 and the storage draft deleted, as the docs rules say.
