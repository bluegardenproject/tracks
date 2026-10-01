# Plan: closing Tracks and reopening its tracks

**Status: built.** Part of the [v2 masterplan](../masterplan.md), the [v2.0.0 scope](../masterplan.md#v200-scope). Today Tracks closes only through `tracks stop` or by killing its tmux server, and when it opens again every track whose window was open is done: its window is gone and the daemon's first look ends it. v1 asks, when its session starts, whether to bring those tracks back. This plan adds **Close Tracks** to Quick Access and v1's question to the start.

## What users get

- **Close Tracks in Quick Access** (`c`): a box asks "Close Tracks?" and says the open tracks can be reopened when Tracks starts again. **Close Tracks** stops the daemon, which finishes the creations under way first, then the tmux server: every window closes and the terminals return to their shell. **Cancel** (Esc) leaves everything as it was. `tracks stop` closes the same way.
- **Reopening on the next start:** a track whose window was open when Tracks closed, or when its tmux server went away (a crash, the machine restarting), is **interrupted**. It's done in Station, and resumable from there as any ended track.
- **When Tracks starts its tmux server and tracks are interrupted,** the terminal lists them with their status and asks "Reopen them? [Y/n]" ("Reopen it?" for one) before attaching, as v1 does. Yes resumes each, oldest first, printing the steps; a track that can't be resumed (its worktree is gone, its agent isn't on the Engines tab) is named with why, and stays done. No, or closing the input, leaves them done and interrupted: the question comes back at the next start. When one track alone came back, its window is shown.
- **Every interrupted track comes back,** whatever its status was: one whose agent had exited comes back with its agent started again.
- Attaching another terminal to a running Tracks doesn't ask, and neither does a terminal that isn't a TTY.

## How it works

- **`track`:** an `Interrupted` event and a `State.Interrupted` flag. Interrupted ends an open track as Ended does, with the flag set; Resumed, Cleaned and Archived clear it. The status stays done.
- **`store`:** an `interrupted` column (migration `0012_interrupted.sql`), saved with the state; `InterruptedTracks` lists the unarchived ones, oldest first.
- **`tracks.Service.Interrupt`**, run by the daemon as it starts, before it serves: every open track without a window is reported Interrupted. Windows only go away with their tmux server while the daemon is down, so the sweep that ends tracks whose window closed is left as it is. A daemon exits when its tmux server's PID changes as well as when the session is gone, so one still running when Tracks opens again within a check doesn't end the tracks the next daemon interrupts. `Interrupted` lists them; `Reopen` resumes each and returns those it reopened and why the others failed.
- **RPC:** `interrupted` and `reopen`, the latter sending each step as Resume does.
- **`cli`:** `start` asks after starting the daemon, only when it created the session and stdin is a terminal. Quick Access's Close Tracks opens a confirm popup (`popup close-tracks`), then closes as `tracks stop` does, through one shared function.
- **`ui/confirm`:** the confirm popup: a title, a line of text, a danger button and Cancel; `y` or Enter confirms, `n` or Esc cancels.

## Packages

- `track`: the event and the flag.
- `store`: the column and `InterruptedTracks`.
- `tracks`: `Interrupt`, `Interrupted`, `Reopen`.
- `daemon`, `rpc`: interrupting on start; exiting when the tmux server restarts; the two calls.
- `tmux`: the server's PID.
- `cli`: the question at start; Close Tracks; `tracks stop` sharing the close.
- `ui/quickaccess`, `ui/confirm`: the entry and the popup.

## Not in this plan

- Asking the agents to exit before the server closes: they get the hangup the server sends, and their sessions are saved as they go.
- A `tracks reopen` command: Station resumes any interrupted track, and the question comes back at the next start.

## Tests

- `track`: Interrupted on an open track and on an ended one; what clears the flag.
- `store`: the column round trip; `InterruptedTracks`; the migration.
- `tracks`: `Interrupt` reports only open tracks without a window; `Reopen` resumes the interrupted ones and reports a failure without stopping.
- `daemon`: the daemon interrupts on start and exits when the server restarts; the two calls' round trip.
- `ui/confirm`, `ui/quickaccess`: keys and clicks.
- `cli`: the question's answers.
- **By hand:** closing from Quick Access with tracks open, starting again and answering yes and no; killing the tmux server instead.
