# Chunk 1: technical groundwork

Part of the [v2 masterplan](../masterplan.md). This file is deleted in the PR that completes the chunk.

## Outcome

`make dev && ./tracks --new-app --demo` opens a v2 session on its own tmux server. It has:
- a placeholder Main window
- several fake track windows (fake agent, real terminal, fake dev-server log)

The installed v1 app keeps running untouched the whole time. Nothing here is visible to v1 users, and release binaries don't contain it.

Out of scope: the footer, menus and the real Main (chunks 2 and 3), real tracks (chunk 5).

## Why the isolation matters

The maintainer uses the installed v1 app (`~/.tracks/tracks`, from `scripts/install.sh`) every day, and builds Tracks from inside Tracks.

Today a local build is unsafe to run. It reports a different version from the installed release: `dev` from `go build`, or the git version from `make build`. [`ensureDaemonUp`](../../../cmd/bootstrap.go) treats a version mismatch as stale and restarts the daemon, which stops every running track.

`make install` has a similar trap. It copies to `~/bin/tracks`, which shadows the installed app if `~/bin` comes first on PATH. During v2 development only `make dev` and `./tracks` are used.

## 1a. Build tag, flag and v1 guards

- **Dispatch in `main.go`:**
  - `v2_on.go` (`//go:build v2`): `--new-app` or `TRACKS_NEW_APP=1` routes to `internal/v2/cli`; everything else goes to v1 as today.
  - `v2_off.go` (`//go:build !v2`), the default for releases: on `--new-app` or `TRACKS_NEW_APP=1`, it exits 1 with "this is a v2 dev session; use ./tracks".
- **`make dev`** runs `go build -tags v2 -o tracks`. `make build`, `build-all` and releases stay untagged.
- **v1 guard: no restarting a foreign daemon.** `ensureDaemonUp` never restarts a daemon whose `ExePath` (already in the ping result) differs from its own binary. It prints "the running daemon belongs to <path>; use --new-app to test this build" and exits. `tracks update` replaces the binary at the same path, so update restarts keep working.
- **Everything reads the environment variable, not the flag:** the flag sets `TRACKS_NEW_APP=1`, and every v2 process reads that variable. The v2 tmux server exports it globally, so panes, popups, key bindings and helper commands stay in v2 without the flag.
- **Tests:**
  - dispatch, including the `!v2` hint
  - the daemon guard as a small pure decision: same path means restart; different path means refuse with the hint

## 1b. `internal/v2` skeleton and agent docs

**Sync first:** the package list below, and the text of both agent files, are agreed before anything is created.

- **Packages for chunks 1 and 2 only:**
  - `internal/v2/cli`: cobra root, `--demo`, `demo stop`, hidden helper commands
  - `internal/v2/platform`: profile, paths, shell helpers
  - `internal/v2/tmux`
  - `internal/v2/demo`
  - `internal/v2/ui/...` (chunk 2)
  - `internal/v2/footer` (chunk 2)

  Everything else from the masterplan layout is created when its chunk starts.
- **Root `AGENTS.md`** (about one page):
  - the v1/v2 split in its first lines
  - the layout map and dependency rules
  - `make dev`, `go test ./...`
  - conventions: file size, tests, doc comments
- **`CLAUDE.md`** is a one-line import of `AGENTS.md`.
- **Profile and paths (`platform`)** are resolved once, before any config, daemon or tmux contact:
  - config: `~/.config/tracks-v2/config.yaml`
  - data: `~/.local/state/tracks-v2/` (worktrees, logs, generated tmux config)
  - daemon socket directory: `tracks-v2-<uid>`
  - tmux socket: `tracks-v2`
  - demo: tmux socket `tracks-v2-demo`, data in a temp directory wiped on every start
- **Global helper files** (Claude skills and agents, the Cursor rule) use `tracks-v2-*` names once v2 writes any, so v1 and v2 never overwrite each other. The v2 proxy port range is separate from v1's.

## 1c. Dedicated tmux server (`internal/v2/tmux`)

It starts as a copy of v1's [`internal/tmux`](../../../internal/tmux/tmux.go). v1's package is not modified.

- **Socket:** the client carries the socket, and every call goes through one builder that adds `-L <socket>`, including `Attach`. Helpers that v1 had outside the package live here: background `run-shell`, `has-session`, `select-window`.

```go
type Client struct{ socket string }

func New(socket string) *Client { return &Client{socket: socket} }

func (c Client) command(args ...string) *exec.Cmd {
	return exec.Command("tmux", append([]string{"-L", c.socket}, args...)...)
}
```

- **Test guard:** in a test binary (`testing.Testing()`), `command` panics unless the socket starts with `tracks-test-`. No test can reach a real Tracks server. This structurally fixes "Bug 7" in [ROADMAP.md](../../ROADMAP.md).
- **`tmuxtest.Socket(t)`** returns a unique `tracks-test-<random>` socket. It skips when tmux is missing, and kills the server in `t.Cleanup`.
- **`Location()`** parses `$TMUX` and returns `outside`, `ownServer` or `otherServer`, with no tmux call.
- **`Available()`** requires tmux 3.2 or newer, parsed from `tmux -V`. Unparseable versions (`master`, `next-3.6`) pass.
- **Generated config** (`conf.go`, `text/template`):
  - written to `<data>/tmux.conf` on every fresh start, and read only when the server starts (`tmux -L <socket> -f <conf> new-session -d ...`)
  - named sections, so chunk 2 adds the footer section
  - contents of the base section:

```tmux
# Generated by tracks on every start. Do not edit.
# Personal overrides: ~/.config/tracks-v2/tmux.conf
set -g default-terminal "tmux-256color"
set -as terminal-features ",*:RGB,*:extkeys"
set -g escape-time 0
set -g extended-keys on
set -g focus-events on
set -g mouse on
set -g set-clipboard on
set -g history-limit 50000
set -g renumber-windows on
set-environment -g TRACKS_NEW_APP 1
{{- if .AtLeast34 }}
set -as terminal-features ",*:hyperlinks"
{{- end }}
source-file -q ~/.config/tracks-v2/tmux.conf
```

- **Start-up rules:** a pure `planStartup(location, sessionExists)` decides:
  - plain terminal, no session: create the session, then attach
  - plain terminal, session running: attach a second client
  - inside a Tracks v2 pane: select window 0 (Main)
  - inside any other tmux: exit 1 with

```text
tracks: you're inside another tmux session.
Tracks runs on its own tmux server. Open a new terminal tab and run it there.
```

## 1d. Demo session

- **`./tracks --new-app --demo`:**
  - wipes the demo data directory and generates the config
  - starts the server on `tracks-v2-demo`
  - creates window 0, `Main`, running a placeholder `tracks main` view that simply says it's a placeholder (the real one comes in chunks 2 and 3)
  - creates 7 fake track windows, then attaches
- **`./tracks --new-app --demo stop`** kills the demo server.
- **Fake track window:**
  - agent pane on the left, running the hidden `tracks demo-agent --scenario <name>`
  - right column (30%) with a real shell in a temp directory
  - on some tracks, a fake dev-server log below it
  - pane titles: `agent`, `terminal`, `web:3000`
- **Scenarios for `demo-agent`:** `running`, `your-turn`, `needs-approval`, `finished`. Each prints scripted, Claude-like output with pauses, so panes look alive without a real agent.
- **Fake tracks** come from `internal/v2/demo`: plain data with names, kinds, repos and statuses. Chunk 2 puts the `Source` interface in front of it.

## Tests

- **Unit, no tmux:**
  - `-L` in every argument list
  - `Location()` against sample `$TMUX` values
  - `planStartup` as a decision table
  - golden files for the generated config (tmux 3.2 and 3.4)
  - `tmux -V` parsing
  - profile and path resolution (v2 paths never equal v1 paths)
  - the daemon guard
- **Integration, on `tmuxtest`:**
  - start a server with the generated config and read options back (catches config errors against the installed tmux)
  - round trip: session, window, split, capture, kill
  - build the demo layout and assert windows, panes and titles
- **Manual:**
  - v1 running normally, then `./tracks --new-app --demo` from a new terminal tab: v1 is unaffected
  - `./tracks --new-app` from inside the v1 tmux: refused
  - a plain `./tracks` (untagged build) with the v1 daemon running: refused with the hint, no restart
  - `demo stop` removes the server

## PR slices

1. **1a:** build tag, dispatch, `make dev`, v1 daemon guard. This is the only chunk-1 PR that touches v1 files, and it can ship in a v1 release.
2. **1b:** `internal/v2` skeleton, `AGENTS.md` and `CLAUDE.md`, profile and paths, after syncing on the layout and text.
3. **1c:** `internal/v2/tmux` with socket, guard, `tmuxtest`, config generation, start-up rules.
4. **1d:** demo session, fake agent, placeholder Main.

After 1d the maintainer plays with the session before chunk 2 starts.
