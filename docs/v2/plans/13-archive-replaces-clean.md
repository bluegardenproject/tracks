# Plan: Archive replaces Clean

**Status: built.** Part of the [v2 masterplan](../masterplan.md), chunks 4 and 5. It changes what ended tracks can do: **End** closes the window, and **Archive** removes the worktrees and local branches, which makes **Clean** unnecessary. It changes [08-resume-and-clean.md](08-resume-and-clean.md), [10-track-status.md](10-track-status.md) and [11-archive-and-filters.md](11-archive-and-filters.md) where they say otherwise.

## What users get

- **End** closes the track's window and agent and keeps its worktrees and branches. The track is **done**.
- **Clean is gone** from the details, its `l` key and the daemon.
- **Archive** (`a`) takes a done track out of Station and removes its worktrees and its local branches. Pushed branches and PRs stay on GitHub. The track is **closed**.
- **Closed means archived:** closed tracks show only under the "Archived only" filter. The filter popup no longer has a "closed" status.
- **Unarchive** (`u`) puts the track back in Station as **done**.
- **Resume** works on an unarchived track. It asks to re-create the worktrees ("Worktree couldn't be found"), and a deleted branch comes back from the pushed branch on origin, or starts again from `origin/<base>` when it was never pushed.

## Archive

- **Before removing anything,** Archive runs the same check as Derail: unsaved work in the worktrees, and commits that exist only on the track's branches. Without any, it asks "Archive rate-bug? Its worktrees and local branches are removed; what was pushed stays." with **Archive** or Cancel; Enter confirms. With some, it lists it and offers **Archive anyway** or Cancel; only `y` confirms.
- **A track without worktrees,** or one whose worktrees are already removed, is archived at once.
- **The branches' names are saved first,** as the agent may have renamed them, so Resume can find them on origin later.
- **Auto-archive** runs the same check. "Tracks with unsaved work" is either **Skip them** (they stay in Station) or **Archive, remove nothing**, which archives them with their worktrees and branches left in place.

## Status

- `State.Status()`: archived gives closed; ended and not archived gives done. `cleaned_at` stays, now meaning Archive removed the worktrees; it no longer sets a status. Resume clears it.
- The filter's SQL: done is `closed_at IS NOT NULL AND archived_at IS NULL`, closed is `archived_at IS NOT NULL`.
- The GitHub poll skips archived tracks instead of cleaned ones.
- No migration: existing cleaned tracks that aren't archived become done, with their worktrees shown as removed.

## Changes

- `workspace`: `Restore` re-creates a deleted branch (`ls-remote` on origin, then fetch and `worktree add -b` from the pushed branch or the base); Derail's `Derail` is renamed `Discard`, shared by Archive and Derail; the worktree-only `Unsaved` check is gone.
- `tracks`: `Archive` checks `Lost` and discards; `Clean` and `Unsaved` are gone; Resume no longer refuses cleaned tracks.
- `rpc`, `daemon`, `cli`: `clean` is gone; `archive` and `derail` answer a `LostResult`.
- `ui/tracksview`: no Clean button; Archive's questions; the re-create question says where a deleted branch comes from. `ui/tracksfilter`: no closed tick. `ui/source`: `Removable` replaces `Cleanable`.

## Tests

- **`workspace`:** Restore from the pushed branch and from the base after Discard, in a real git repo.
- **`track`, `store`:** closed only when archived; the filter's done and closed.
- **`tracks`:** Archive with work that would be lost, forced, keeping renamed branches, twice; Unarchive gives done; Resume of an unarchived track; auto-archive skipping and keeping.
- **`ui`:** Archive's questions and keys; an unarchived track shows done and can be resumed; the re-create question for a removed branch; the filter popup without closed.
- **By hand:** archive a track with an unpushed commit (asked), push and archive (removed); unarchive and resume it.

## Delivery

One branch, **`feat/v2-archive-removes`**, stacked on `feat/v2-station-order`.
