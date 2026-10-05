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
  | Review | Lazily, right before the first dev server starts, or on `tracks setup` |
  | Ask, Plan, Doc | Never: they have no worktree and work in the main checkout |

  A Work track also starts it when promoted from Ask or Plan, when a repo
  is added to it, and when it's resumed; resuming forgets the setup of a
  worktree it re-creates.

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
- The pane waits for setup (`tracks setup --wait`), then runs the command in
  `<worktree>/<dir>` with `PORT` set and `{{port}}` replaced. `tracks logs`
  reads the pane's scrollback; there's no log file.
- Dev servers run only in repos with a worktree (Work and Review tracks).
- The pane owns the process; its first process leads the process group,
  which tmux reports as `pane_pid` when it's time to stop.
- Teardown signals the whole group (SIGTERM, then SIGKILL after 5 s). It runs
  on `tracks down`, whenever Tracks closes a track window, and on `tracks
  stop`. Node servers fork workers, so killing the pane alone isn't enough.
- Later: a window closed outside Tracks leaves its servers only tmux's
  SIGHUP, and nothing yet cleans up servers orphaned by a crash.
- Restarting the daemon leaves panes, and their servers, running.
- A crashed server isn't restarted. Its pane stays open on the exit, so the
  user sees why. See Errors.

### Ports

- **Assigned (default):** each track claims a block of 10 ports in 20000–29999
  when its first such server starts (`track_ports`); an ended track's block
  goes to the next track that needs one. A server gets the next port in the
  block, passed as `$PORT` and `{{port}}`. The port is known before the server
  starts.
- **Fixed:** for servers that ignore `$PORT`. Only one track can run it at a
  time; a second start is refused and names the track that holds the port.
- **Detect:** the port is read from what the server's processes listen on.

### Detection

- When servers are listed (`tracks servers`, the Proxy tab), one `ps` and one
  `lsof -iTCP -sTCP:LISTEN` give every process and the ports it listens on,
  and each track pane's process tree is walked. No poll loop: a list costs
  one `ps` and one `lsof`, about 100 ms here, more on a busy machine.
- A listener under a dev-server pane belongs to that server. Its own port
  listening makes it ready; a detect-mode server takes the first port found.
- Any other listener becomes an unnamed server (`:5173 (vite)`), so servers
  an agent or a terminal starts show up as proxy inputs too.
- In an agent pane, Tracks finds the agent process (`claude`, `agent`) and
  leaves out its own port (Claude Code listens on one) and everything it
  starts directly, such as MCP servers. What it runs through a shell (its
  commands) counts. Once the agent exited, the pane's shell counts like a
  terminal's.
- Type, when unset, is guessed from the last part of each word of the
  listener's command line: a `metro`, `rspack`, `rsbuild`, `vite`, `webpack`,
  `next`, `storybook`, `electron`, `astro`, `nuxt` or `remix` word (`react-
  native start` is Metro), else the program's name (`node`, `python3`). The UI
  shows a guessed type dimmer.

### State of a running server

`starting` (the pane runs, nothing in it listens yet) → `ready` (something
in it listens) → `exited` or `crashed` when the server ends with zero or
another code; `stopped` without a pane. Readiness is the listener alone;
matching a log line can come later.

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

- Runs in the daemon, which brings it in step every 2 s and right after a
  change; it reads the processes only while a port has an input.
- Each output port listens on 127.0.0.1 and ::1 only, never on the LAN, with
  an `httputil.ReverseProxy` to `localhost:<port>`, which reaches a server on
  either loopback (Vite on macOS often listens on ::1 alone).
  `FlushInterval: -1`, and WebSocket upgrades pass, so HMR works. The
  client's `Host` header is kept, so a dev server builds its URLs for the
  output port. That covers node, Metro, rspack, Electron's renderer and the
  rest; raw TCP waits for a real need.
- Binds lazily: an output port is listened on only while it has an input. An
  output port with no input holds nothing, so defining `8081` doesn't squat
  Metro.
- If another program holds the port (say a server the user started by hand),
  the tab shows it as blocked, with the holder's process, and Tracks tries
  again on the next sync. The holder comes from `lsof`: on macOS a loopback
  bind succeeds next to a server on every address, so the bind alone can't
  tell. A port whose input listens on the port itself is blocked too.
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
