# Chunk 1: technical groundwork

Part of the [v2 masterplan](../masterplan.md). This file is deleted in the PR that completes the chunk.

## Outcome

`make dev && ./tracks --new-app --demo` opens a v2 session on its own tmux server. It has:
- a placeholder Tracks window, the first screen on the Charm v2 TUI stack
- several fake track windows (fake agent, real terminal, fake dev-server log)

The installed v1 app keeps running untouched the whole time. Nothing here is visible to v1 users, and release binaries don't contain it.

Out of scope: the footer, menus and the real Tracks window (chunks 2 and 3), real tracks (chunk 5).

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

- **Packages for chunk 1 (agreed):**
  - `internal/v2/cli`: commands: the root, `--demo`, `demo stop`, hidden helpers (the fake agent, the placeholder view)
  - `internal/v2/platform`: profile and paths
  - `internal/v2/tmux`: tmux client bound to one socket, the generated config, start-up rules, version check
  - `internal/v2/tmux/tmuxtest`: throwaway tmux servers for tests
  - `internal/v2/demo`: fake tracks, fake agent scenarios, building the demo session
  - `internal/v2/theme`: design tokens and theme values
  - `internal/v2/ui/style`: Lip Gloss styles built from tokens
  - `internal/v2/ui/tracksview`: the placeholder Tracks window

  Everything else from the masterplan layout is created when its chunk starts.
- **Root `AGENTS.md`** (about one page):
  - the v1/v2 split in its first lines
  - the layout map and dependency rules
  - `make dev`, `go test ./...`
  - conventions: file size, tests, doc comments, colours only through theme tokens
- **`CLAUDE.md`** is a one-line import of `AGENTS.md`.
- **Profile and paths (`platform`)** are resolved once, before any config, daemon or tmux contact:
  - config: `~/.config/tracks-v2/config.yaml`
  - data: `~/.local/state/tracks-v2/` (worktrees, logs, generated tmux config)
  - tmux socket: `tracks-v2`
  - no daemon socket yet: v2 runs without a daemon until chunk 5
  - demo: tmux socket `tracks-v2-demo`, data in a temp directory wiped on every start
- **Global helper files** (Claude skills and agents, the Cursor rule) use `tracks-v2-*` names once v2 writes any, so v1 and v2 never overwrite each other. The v2 proxy port range is separate from v1's.

## 1c. Dedicated tmux server (`internal/v2/tmux`)

Written fresh, with v1's [`internal/tmux`](../../../internal/tmux/tmux.go) as a reference. v1's package is not modified. Methods are added when a caller needs them.

- **Socket:** the client carries the socket, and every call goes through one argument builder that adds `-L <socket>`, including `Attach`.
- **Test guard:** in a test binary (`testing.Testing()`), the builder panics unless the socket starts with `tracks-test-`. No test can reach a real Tracks server. This structurally fixes "Bug 7" in [ROADMAP.md](../../ROADMAP.md).
- **`tmuxtest.Socket(t)`** returns a unique `tracks-test-<random>` socket. It skips when tmux is missing. In `t.Cleanup` it kills the server and removes the socket file, which tmux leaves behind.
- **`LocationOf($TMUX, socket)`** returns `Outside`, `OwnServer` or `OtherServer`, with no tmux call.
- **`InstalledVersion()`** requires tmux 3.2 or newer, parsed from `tmux -V` (`next-3.6` reads as 3.6). Unparseable versions (`master`) pass.
- **Generated config** (`conf.go`, `text/template`):
  - written to `<data>/tmux.conf` on every fresh start, and read only when the server starts (`tmux -L <socket> -f <conf> new-session -d ...`)
  - `default-terminal` falls back to `screen-256color` when the `tmux-256color` terminfo entry is missing
  - v1's `TRACKS_ID` and `TRACKS_SOCKET_DIR` are removed from the server environment
  - 24-bit colour: apps in panes always see `COLORTERM=truecolor`, and tmux converts per attached terminal. tmux sends 24-bit colour only to the terminal type that started the server, and only if it reported `COLORTERM=truecolor`. Two terminals with the same `TERM` but different colour support (iTerm2 and older Terminal.app both use `xterm-256color`) can't be told apart; the override file can fix that.
  - chunk 2 adds the footer settings
  - contents today:

```tmux
# Generated by tracks on every start. Do not edit.
# Personal overrides: ~/.config/tracks-v2/tmux.conf
set -g default-terminal "tmux-256color"
set -as terminal-features ",*:extkeys"
{{- if and .TrueColor .OuterTerm}}
set -as terminal-features ",{{.OuterTerm}}:RGB"
{{- end}}
set -g escape-time 0
set -g extended-keys on
set -g focus-events on
set -g mouse on
set -g set-clipboard on
set -g history-limit 50000
set -g renumber-windows on
set-environment -g COLORTERM truecolor
set-environment -g TRACKS_NEW_APP 1
set-environment -gu TRACKS_ID
set-environment -gu TRACKS_SOCKET_DIR
{{- if .Version.AtLeast (v 3 4)}}
set -as terminal-features ",*:hyperlinks"
{{- end}}
source-file -q "~/.config/tracks-v2/tmux.conf"
```

- **Start-up rules** (in `cli`): a pure `planStartup(location, sessionExists)` decides:
  - plain terminal, no session: create the session, then attach
  - plain terminal, session running: attach a second client
  - inside a Tracks v2 pane: select window 0 (Tracks)
  - inside any other tmux: exit 1 with

```text
error: you're inside another tmux session.
Tracks runs on its own tmux server. Open a new terminal tab and run it there.
```

- **`./tracks --new-app stop`** kills the v2 server. Until the Tracks window exists, window 0 runs a hidden placeholder command.

## 1d. Demo session

- **`./tracks --new-app --demo`:**
  - wipes the demo data directory and generates the config
  - starts the server on `tracks-v2-demo`
  - creates window 0, `Tracks`, running a placeholder view (a hidden command) that simply says it's a placeholder (the real one comes in chunks 2 and 3)
  - creates 7 fake track windows, then attaches
- **`./tracks --new-app --demo stop`** kills the demo server.
- **Fake track window:**
  - agent pane on the left, running the hidden `tracks demo-agent --scenario <name>`
  - right column (30%) with a real shell in a temp directory
  - on some tracks, a fake dev-server log below it
  - pane titles: `agent`, `terminal`, `web:3000`
- **Scenarios for `demo-agent`:** `running`, `your-turn`, `needs-approval`, `finished`. Each prints scripted, Claude-like output with pauses, so panes look alive without a real agent.
- **Fake tracks** come from `internal/v2/demo`: plain data with names, kinds, repos and statuses. Chunk 2 puts the `Source` interface in front of it.

## 1e. TUI stack (Charm v2)

- **Libraries:** `charm.land/bubbletea/v2`, `lipgloss/v2`, `bubbles/v2`, `huh/v2` and `bubblezone/v2`, next to v1's Charm v1 modules in the same `go.mod`. v1 keeps its imports. Each module is added in the PR that first imports it, since `go mod tidy` drops unused ones.
- **The placeholder Tracks window is the first Bubble Tea v2 program.** It proves the stack inside the v2 tmux server before chunk 2 builds on it:
  - alternate screen and resizing (done)
  - true colour through the generated tmux config (done)
  - keys, including Alt combinations and extended keys (with the first interactive screen)
  - mouse clicks, through a `bubblezone` target (with the first interactive screen)
- **24-bit colour inside the server:** Tracks screens always render with the 24-bit profile (`style.Profile()`), unless `NO_COLOR` is set, and tmux converts per attached terminal. Detection alone would pick 256 colours: inside tmux it asks the attached terminal, and window 0 starts before any terminal attaches.
- **Checks:** an untagged release build links none of it (`go version -m tracks`), and v1's screens are unchanged. Adding the modules raised some indirect dependencies v1 shares (`x/ansi`, `colorprofile`, `go-runewidth`, ...); v1's tests pass with them.

## 1f. Design tokens and the first theme (`internal/v2/theme`)

- **Tokens are the only way to a colour.** A token names a purpose, for example `text.muted` or `bg.hover`. Screens, the footer and the status model use tokens, never colour values.
- **The package has no UI library imports.** It declares the token names, holds theme values as plain strings, and validates them. Adapters turn values into colours at the edges: `ui/style` for Lip Gloss (chunk 1 needs only what the placeholder uses; Huh comes in chunk 2), and the footer renderer for tmux (chunk 2).
- **First token set,** extended as screens need more:

  | Group | Tokens |
  |---|---|
  | text | `text.default`, `text.muted`, `text.faint`, `text.inverse`, `text.accent` |
  | background | `bg.base`, `bg.surface`, `bg.overlay`, `bg.selected`, `bg.hover` |
  | border | `border.default`, `border.focus` |
  | emphasis | `accent`, `highlight` |
  | state | `state.success`, `state.warning`, `state.danger`, `state.info` |

- **Values:** `#rrggbb`, each with a dark and a light variant. The variant is picked from the terminal's background colour, which Bubble Tea v2 reports; dark is the default. Terminals with fewer colours get the nearest match.
- **The first theme is built in:** `theme/themes/default.yaml`, embedded in the binary, so colour values stay out of Go code. Loading theme files from the config directory comes later and reuses the same parser and validation.
- **Adding a token** is one new name in `theme/tokens.go` plus a value in every theme file. Parsing fails on a missing or unknown token, or a value that isn't `#rrggbb`.
- **The Tracks window shows every token as a swatch,** so the theme can be judged in the real terminal.
- **Guard test:** a test scans `internal/v2` and fails on colour literals outside `theme` (hex strings, `lipgloss.Color(...)`, tmux `fg=`/`bg=` values, raw ANSI colour escapes). `AGENTS.md` states the rule.

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
4. **1d, 1e and 1f:** demo session, fake agent, design tokens with the first theme, and the placeholder Tracks window on Charm v2.

The docs changes made during chunk 1 ship with its PRs. After the last slice the maintainer plays with the session before chunk 2 starts.
