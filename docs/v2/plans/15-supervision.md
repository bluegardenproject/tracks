# Plan: supervision

**Status: built.** Part of the [v2 masterplan](../masterplan.md), chunk 5's supervision and the [v2.0.0 scope](../masterplan.md#v200-scope). When a track's agent exits, its pane falls back to a login shell, and nothing in Station says so: the track stays **active** while nobody works in it. This plan adds two track statuses for it, and a way to start the agent again in place.

## What users get

- **An agent that fails** (exits with a non-zero code, crashes, or can't start, such as a binary that isn't found) puts its track in **error**, drawn in the danger colour. It needs attention: the footer's track slot gets its badge, as for action required.
- **An agent the user quits** (`/exit`, exit code 0) puts its track in **agent exited**, drawn as a warning, without attention.
- **Restart** (`r`) in the details of an open track in either status, after End, starts the agent again in the same pane, on the same session, with the progress in the notice line, then "Restarted rate-bug." and a switch to the track. The window, its terminals and its number stay.
- **`tracks restart [<id>]`** does the same from the command line. Without an ID it's the track the command runs in, so typing `tracks restart` in the shell the agent left behind brings it back, even in the 2 seconds before Station shows the exit.
- **Refused:** a track whose agent is still running ("rate-bug's agent is still running."), an ended one ("rate-bug has ended: resume it instead."), and one whose engine isn't added ("Add Cursor on the Engines tab to restart this track.").

## The statuses

Two new entries in `track/status.go`:

| ID | Label | When | Badge | Attention |
|---|---|---|---|---|
| `error` | error | the agent exited with a non-zero code | danger | yes |
| `exited` | agent exited | the agent exited with code 0 | warning | no |

- **Priority:** error is shown first, before action required; agent exited comes after action required and before active.
- **A new badge state,** `danger`, drawn in `state.danger.bg` and `state.danger.text`.

## How it works

- **The pane records the exit.** The wrapper that runs the agent (`agents.Wrapper`) saves the agent's exit code in the window option `@tracks_exit`, with `tmux set-option -w -t "$TMUX_PANE"`, before it starts the login shell. tmux keeps it as long as the window lives, so a daemon that was down when the agent exited still sees it.
- **The daemon's tick reads it.** Every 2 seconds, after the sweep and the screen check, `CheckExits` lists the windows, which reads `@tracks_exit` in the same `list-windows` call. For an open track whose window has the option, it reports `agent.exited` (code 0) or `agent.failed` (any other code), once per code, and the daemon logs the code: "rate-bug's agent exited with code 127".
- **State:** `track.State` gets `Exit`, `""`, `exited` or `failed`, stored in a new `tracks.agent_exit` column. `Apply` sets it on the two new events while the track is open, and clears it on `created` and `resumed`. Agent hook events on an exited track are dropped: the window option would set it again at the next sweep anyway.
- **Restart** reuses Resume's steps without the worktrees: the per-track hooks, the engine's resume command, and `trackwin.Respawn`, which clears `@tracks_exit` and restarts the agent pane with `respawn-pane -k`. It then reports `resumed`. A Claude session that never started (its transcript doesn't exist, as when the binary wasn't found) starts fresh on the original prompt instead of resuming; `Engine.Started` says which. Cursor chats always exist, since Tracks creates them before the agent runs.
- **Station's status filter** gets the two statuses, and active and action required leave out the tracks whose agent exited.

## Packages

- `agents`: the wrapper's exit line.
- `tmux`: `@tracks_exit` in `ListWindows`; `trackwin`: `Info.Exit`, and `Respawn` clears it.
- `track`: the two statuses, the events, `State.Exit`, the danger badge.
- `store`: the migration, reading and writing `agent_exit`.
- `tracks`: `CheckExits`; `Restart`; `Engine.Started`.
- `rpc`, `daemon`, `cli`: the `restart` method and command.
- `ui/tracksview`: the Restart button; the danger badge in Station and the details.

## Not in this plan

- Notifications when a track fails: they come with desktop notifications.
- Restarting a failing agent on its own, and a limit on restarts.
- Telling a crash from a failed start: both are an error.

## Tests

- `agents`: the parity tests expect v1's command plus the exit line.
- `trackwin`: on a real tmux server, a wrapped agent that exits with 3 leaves `3` in the window list and its pane at a shell; `Respawn` clears it.
- `track`: `Apply` for the new events from every status; `Status` and the priority order.
- `store`: the migration and the column.
- `tracks`: `CheckExits` reports each exit once, again when the code changes, and not for ended tracks; Restart respawns with the resume command, starts fresh without a transcript, and each refusal.
- `ui/tracksview`: the badges, and the Restart button only on exited and failed tracks.
- **By hand:** `/exit` in a Claude track, `kill` of the agent process, a track whose engine binary is renamed; Restart from Station and from the shell.
