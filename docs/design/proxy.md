# Proxy and dev servers

**Status:** planning. The UI comes separately; this doc covers how it works
underneath. v1's design (`docs/design/dev-servers.md`, in git history before
`a8a2602`) is the starting point, not the template.

## Problem

A track's work can't be run without merging it. Tracks should start a repo's
dev servers inside a track, and give the user a fixed port to point a browser,
a desktop app or a simulator at, whichever track is behind it.

Two constraints: tracks never collide with each other, and never with servers
the user starts by hand on default ports.

## What v1 taught us

- **Headless processes failed.** `tracks up` blocked through `pnpm install` and
  readiness, overran Claude's command timeout, and the logs were easy to miss.
  v1 moved each server into a pane that owns it. v2 keeps that.
- **Half-wired features.** `ready:` and `post_start` were accepted but never ran
  for weeks. Ship less, and wire all of it.
- **Claude bypassed Tracks.** When `tracks up` failed (it dialled the wrong
  socket), Claude ran `pnpm dev` itself and the proxy never saw it. v2 detects
  servers it didn't start.
- **The proxy itself worked:** a fixed port that is bound lazily, an upstream
  you can switch, WebSockets and HMR passed through.

## Concepts

| Concept | Defined in | What it is |
|---|---|---|
| Setup | Repositories tab, per repo, optional | One shell command run once per worktree, right before its first dev server (`pnpm install`) |
| Dev server | Repositories tab, per repo, optional, any number | A named command that serves on a port |
| Running server | Discovered at runtime | A process listening on a port inside a track: a dev server Tracks started, or one found by detection |
| Output port | Proxy tab | A fixed port on localhost (`3000`). Its input is one running server, or none. |

## Repo settings

The repo form keeps Name, Path, Base branch and Draft PRs. Below them are two
buttons, **Add setup** and **Add servers**. A repo that needs neither never sees
more. Clicking one opens that section's inputs. Removing a section's last entry
collapses it back to its button.

The sections use the form's existing fields and buttons: a labelled input with
a hint under it, as Name, Path and Base branch have. Each dev server is a group
of those fields with a **Remove server** button. **Add server** sits under the
last group, the way **Add new proxy port** does on the Proxy tab. Port mode is a
choice that cycles like Draft PRs' toggle; the number input shows only for
fixed. The look gets revisited once it's in use.

### Setup

| Field | Required | Example |
|---|---|---|
| Command | yes | `pnpm install` |

- Runs once per worktree, in the worktree's root. It is the first part of the
  ROADMAP's "Worktree setup".
- When it runs depends on the track's kind:

  | Kind | Setup runs |
  |---|---|
  | Work | On track creation, in the background, while the agent starts |
  | Review, Ask, Plan, Doc | Lazily, right before the first dev server starts, or on `tracks setup` |

  Nothing blocks the agent: setup runs in a `setup` pane in the right column,
  and the pane closes on success. A failed setup keeps its pane open with the
  error.
- The agent works in the worktree while setup runs. Its prompt says so: wait
  with `tracks setup --wait` before building, testing or installing, so it
  never runs against a half-installed tree or starts a second install.
- `tracks up` and the Proxy tab wait for setup too.
- **`.env` files:** a track whose repo has a setup or dev servers gets the
  repo's `.env` files copied into its worktree on creation, whatever the kind;
  copying is cheap. The
  files are the ignored `.env` and `.env.*` files of the main checkout, at any
  depth (monorepos keep one per app), skipping ignored directories such as
  `node_modules`. They keep their relative paths, and an existing file in the
  worktree is never overwritten.
- Done is recorded per track and repo. A failed setup isn't recorded; the next
  `tracks up` or `tracks setup` runs it again. Several servers starting at once
  share one run.

### Dev servers

| Field | Required | Example | Notes |
|---|---|---|---|
| Name | yes | `web` | Unique within the repo |
| Command | yes | `pnpm dev --port $PORT` | One shell line; `&&` for chains |
| Directory | no | `apps/web` | Relative to the repo; default the root |
| Port | yes | assigned · fixed `8081` · detect | See Ports |
| Type | no | `rspack` | A display label; suggestions offered, free text allowed |

## Mechanics

### Process ownership

- `tracks up` opens a `RoleDevServer` pane in the track window's right column
  (`trackwin` already has the role) and returns at once.
- The pane waits for setup, then runs `cd <dir> && <command>` with `PORT` set,
  its output teed to a log file.
- The pane owns the process; the daemon records the pane's process group.
- Teardown signals the whole group (SIGTERM, then SIGKILL). It runs on
  `tracks down`, on track end, and when recovery finds a server whose track is
  gone. Node servers fork workers and watchman, so killing the pane alone isn't
  enough.
- Restarting the daemon leaves panes, and their servers, running.
- A crashed server isn't restarted. Its pane stays open on the exit, so the
  user sees why. See Errors.

### Ports

- **Assigned (default):** each track gets a block of ports in 20000–29999,
  stored on the track. A server gets the next port in the block, passed as
  `$PORT` and `{{port}}`. The port is known before the server starts.
- **Fixed:** for servers that ignore `$PORT`. Only one track can run it at a
  time; a second start is refused and names the track that holds the port.
- **Detect:** the port is read from what the server's processes listen on.

### Detection

- The daemon's poll lists the TCP listeners of every pane process tree in every
  track window, agent panes included, using `lsof -a -p <pids> -iTCP
  -sTCP:LISTEN`.
- A listener under a dev-server pane belongs to that server. Any other listener
  becomes an unnamed running server (`track-x · :5173`), so servers Claude starts
  on its own show up as proxy inputs too.
- Type, when unset, is inferred from the listener's command line: an
  `rspack`/`vite`/`webpack`/`next`/`electron` binary, `react-native start` or
  `metro` for Metro, and otherwise the program name (`node`, `bun`, `python`).
  The UI shows an inferred type dimmer than a configured one.

### State of a running server

`starting` (the pane is up, the port isn't listening yet) → `ready` (the port
answers) → `stopped` or `crashed` (the pane exited with zero, or non-zero).
Readiness is the port check alone; matching a log line can come later.

### Errors

A crashed server or a failed setup may be expected (it might need other work
first), so the user decides what happens next, never Tracks or the agent.

- The daemon detects both from the pane's exit, and sets an error on the track:
  **Server error** (naming the server) or **Setup error**.
- The error is a badge of its own on the Station, next to the track status and
  the PR status, the way the PR status already is. The agent may be working
  fine while its server is down, so it doesn't replace the track status.
  It's also not the track status `error` (the agent failed), which stays as it
  is.
- The agent can read the output with `tracks logs <server>` and tell the user
  what it sees. It doesn't restart, reinstall or fix anything on its own, and
  runs `tracks up` or `tracks setup` only when the user asks. The prompt says so.
- The error clears when:
  - the server or setup is started again (by the user, or the agent at the
    user's request) and doesn't fail;
  - the server is stopped with `tracks down`;
  - the user dismisses it with **Dismiss track errors** in the track's details
    on the Station. The button shows only while the track has an error, on a row
    of its own below the other buttons, with an empty row between.

### The proxy

- Runs in the daemon. Each output port has a `httputil.ReverseProxy` to its
  input's `127.0.0.1:<port>`, with `FlushInterval: -1`, so WebSockets and HMR
  work. That covers node, Metro, rspack, Electron's renderer and the rest; raw
  TCP waits for a real need.
- Binds lazily: an output port is listened on only while it has an input. An
  output port with no input holds nothing, so defining `8081` doesn't squat
  Metro.
- If the port is taken (say a server the user started by hand), the tab shows
  it as blocked, with the holder's process when `lsof` finds one, and retries on
  the next poll.
- An input whose server stops leaves the output port selected but answering 503
  "no server". It isn't cleared, so restarting the server reconnects on its own.
- Output ports and their selected inputs are stored in the database and
  restored when the daemon starts, because the servers outlive the daemon.
- Switching is manual only. Starting a server never moves an output port, even
  when the same server on another track is its input; the new server just
  appears in the selector. (v1 switched automatically.)
- An input is stored by identity: track, repo and server name for a defined
  server, track and port for a detected one.

## Proxy tab

A new tab, between Station and Repositories.

```
╭─ Ports ─────────────────────────────────────────────────────────────╮
│ ╭─────────────────────────────────────────────────────────────────╮ │
│ │  [ 3000 ]          forwards to         [ web · rspack · … ▾ ] × │ │
│ ╰─────────────────────────────────────────────────────────────────╯ │
│ ╭─────────────────────────────────────────────────────────────────╮ │
│ │  [ 8081 ]          forwards to         [ none             ▾ ] × │ │
│ ╰─────────────────────────────────────────────────────────────────╯ │
│                                                                     │
│  [ Add new proxy port ]                                             │
╰─────────────────────────────────────────────────────────────────────╯
```

- One big frame, titled **Ports**, holds everything. Its only content at first
  is the **Add new proxy port** button.
- Adding a port adds a full-width frame for it, and the button moves below the
  last frame, so it's always under the list.
- A port's frame, left to right:
  - left: the output port, an input the user types (`3000`);
  - centre: the port's state: "forwards to" while forwarding or set to
    **none**, "no server ⚠" when the chosen server stopped, "blocked by node
    (pid 1234)" when another process holds the port;
  - right: the input, a dropdown of running servers. It opens the existing
    `widget.Picker`, whose rows read `web · rspack · track-x · :20013`. The first
    entry is **none**.
  - far right: **×** removes the port; so does Delete on a focused port's frame.
    No confirmation, since nothing is lost.
- The port input accepts 1024–65535, not a port already listed, and not one in
  the range Tracks assigns to dev servers (20000–29999). The error shows under the
  input.
- Many ports scroll inside the Ports frame; the add button scrolls with them as
  the last row.
- A new port starts with input **none**, so it binds nothing until a server is
  chosen.
- The tab title gets a green `●` (`Proxy ●`) while at least one port is
  forwarding to a server that's running. It's drawn with the
  `state.success.text` token, not a new colour. A port that is blocked, or whose
  server stopped, doesn't count.

## Data

New migrations, never edits:

- `repos.setup`: the setup command, `''` for none
- `dev_servers (repo_id, position, name, command, dir, port_mode, port, type)`
- `track_ports (track_id, base)`, plus a done marker for setup per track and repo
- `track_errors (track_id, kind, server, at)`: server and setup errors until
  cleared
- `proxy_ports (port, input_track, input_repo, input_server, input_port)`

Running servers aren't stored: the daemon derives them from panes and listeners
each poll.

## Control surface

- RPC: `up`, `down`, `servers` (running servers, defined and detected),
  `proxies`, `proxy.add`, `proxy.remove`, `proxy.select`.
- CLI: `tracks up [server]` (no argument starts all the track's servers),
  `tracks down [server]`, `tracks servers`, `tracks url <server>`,
  `tracks logs <server>`, `tracks setup [--wait]` (runs setup, or waits for the
  running one). The CLI honours `TRACKS_SOCKET_DIR` (v1's bug).
- Proxy tab: output ports, each with its input selector. Rows read
  `web · rspack · track-x · :20013`. The design comes separately.
- Prompts: the work and review prompts learn about `tracks setup --wait`,
  `up`, `down` and `logs`, and that errors are reported to the user, not acted
  on. Changing their golden files is agreed (2026-10-04).

## Order

1. Migrations; Setup and Dev servers on the repo form.
2. Setup by track kind; `tracks setup [--wait]`.
3. Port blocks; `up`/`down`/`logs` in panes; teardown on every path.
4. Detection and running-server state; `servers` RPC and CLI.
5. Proxy in the daemon; stored output ports; the Proxy tab.
6. Prompts.

## Open

- Submodules: later.
