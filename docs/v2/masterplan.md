# Tracks v2 masterplan

Tracks v2 is the next major version of Tracks: a user-friendly terminal app that runs CLI agents (Claude Code, Cursor) in isolated worktrees, with one always-present **Main** window as the command center and one window per track.

This file is the single source of truth for direction, decisions and status. Implementation detail lives in feature plans under [`plans/`](plans/).

## How these docs work

- **`masterplan.md`** (this file) holds the goal, decisions, rules and the chunk list. It is updated whenever a decision changes, and it stays until the v2.0.0 release.
- **`plans/NN-<name>.md`** is the implementation plan for one chunk: tasks, tests, PR slices. It is written when the chunk is designed, and **deleted in the PR that completes the chunk**, since git history keeps it. Knowledge that stays useful moves into code docs (`doc.go`, section `AGENTS.md`).
- **`plans/drafts/`** holds designs already worked out for later chunks. A draft becomes a feature plan when its chunk comes up.

## Chunks

| # | Chunk | Plan | Status |
|---|---|---|---|
| 1 | Technical groundwork: `--new-app`, isolation, `internal/v2` skeleton, dedicated tmux server, demo session | [01-technical-groundwork.md](plans/01-technical-groundwork.md) | planned |
| 2 | Global app layout: Main window (placeholder), track windows, footer navigation, menus, on demo data | [02-app-layout.md](plans/02-app-layout.md) | planned |
| 3 | Main window layout: header and tab navigation, no tab content yet | not written yet | to be designed |
| 4 | Storage: SQLite, list queries, auto-archive, change stream | [drafts/storage.md](plans/drafts/storage.md) | draft |
| 5 | Real tracks: v2 daemon, agents, create/end/resume, supervision | not written yet | to be designed |
| 6 | Agent hooks instead of screen polling | [drafts/hooks.md](plans/drafts/hooks.md) | draft |
| 7 | Main content: Tracks, Repositories, Proxy, Settings tabs, track actions | not written yet | to be designed |
| 8 | v2.0.0 release: delete v1, move `internal/v2` up, drop flag and build tag | not written yet | later |

Chunks 1 to 3 come first, in order. After them, the order of 4 to 7 is decided by what the layout work shows.

## Decisions

- **v2 is a release milestone, not a separate codebase.** It lives in this repo on `main`, next to v1, and is tagged v2.0.0 when done. It doesn't need to be compatible with v1 data or config; importing v1 tracks is undecided.
- **Engine: tmux stays.** It renders agent TUIs faithfully, splits panes next to an agent, keeps sessions alive when the UI closes, and works over SSH.
- **Dedicated tmux server:** v2 runs on its own socket with a config it generates. The user's personal `~/.tmux.conf` and other sessions are never involved.
- **Start from a plain terminal:** started inside another tmux, Tracks refuses with a clear message instead of nesting. The prefix stays Ctrl+b; Alt shortcuts cover common actions without it.
- **UI: Charm v2** (`charm.land/bubbletea/v2`, `lipgloss/v2`, `bubbles/v2`, `huh/v2`, `bubblezone/v2`). The import paths differ from v1's, so both coexist in one `go.mod`.
- **Main is window 0,** always present, the command center with tabs. v2 has no Dashboard window.
- **Track navigation: a fixed footer** (the tmux status line) on every window, with clickable track slots, first/previous/next/last buttons, attention badges and Alt shortcuts. Hover is not possible in the tmux status line and is accepted as missing.
- **Popups:** a full menu (`Alt+p`) and a quick switcher (`Alt+s`).
- **Storage: SQLite** (pure Go, `modernc.org/sqlite`) for tracks, history and the event timeline. `config.yaml` stays a hand-edited YAML file.
- **Agent status from hooks,** not screen polling, with a narrow polling fallback. One direction for now: agent to Tracks.
- **The layout is proven on fake data first:** `./tracks --new-app --demo` opens a playground with fake tracks. The UI is built in its real packages against a data interface, so the playground becomes the product.

## Target shape

```mermaid
flowchart TD
    cli["tracks CLI"] -->|"attach, own socket"| server["tmux server: own socket + generated config"]
    daemon["tracks daemon"] -->|"spawn panes"| server
    server --> mainWin["window 0: Main (Bubble Tea)"]
    server --> trackWin["track windows: agent + terminal + dev-server panes"]
    server --> footer["footer on every window: track navigation + info"]
    trackWin -->|"agent hooks"| daemon
    daemon -->|"only writer"| db["SQLite"]
    mainWin -->|"queries + change stream"| daemon
```

## v1 and v2 on `main`

v1 keeps shipping from `main`, with urgent fixes at any time, while v2 is merged step by step. Rules:

- **Separate tree:** all v2 code lives under `internal/v2/`, including its own CLI root. Everything outside it is v1.
  - v2 PRs don't change v1 packages. The exceptions are small guard fixes, and moving library-neutral helpers into shared packages.
  - v1 fixes don't touch `internal/v2/`.
- **Reuse of v1 packages:** v2 may import stable v1 leaf packages unchanged (`git`, `shellx`, `ports`, ...). If v2 needs different behaviour, it copies the package into `internal/v2/` and changes the copy.
- **Release binaries contain no v2:** v2 is only linked in with the `v2` build tag (`make dev`). Releases, `make build` and `tracks update` stay pure v1. `go test ./...` still tests the v2 packages.
- **Runtime isolation in v2 mode:**
  - separate config, data directory, daemon socket and tmux server
  - namespaced global helper files, and a separate proxy port range
  - a v2 session never touches the installed v1 app, and vice versa
- **Dependencies:** v2-only dependencies don't affect v1. Bumping a dependency v1 also uses needs green v1 tests.
- **CI** runs the full suite on every PR.
- **At v2.0.0:** v1 code is deleted, `internal/v2/*` moves up one level in one mechanical commit, and the build tag and `--new-app` flag are removed.

## Architecture and conventions

**Status: DRAFT, to be agreed.** Every new package group, and every `AGENTS.md`, is proposed and agreed before it's created.

### Principles

- **One package, one purpose,** describable in one sentence without "and".
- **Files around 400 lines at most.** Past that, split by responsibility. It's a prompt to rethink, not a hard rule. v1's 1,000 to 1,600-line files (`state.go`, `handlers.go`, `dashboard.go`, `supervisor.go`, `settings.go`) are the counter-example.
- **Reuse when there's a second real caller,** not in advance. Interfaces are small and live where they're consumed.
- **Pure core, thin edges:** decisions (track lifecycle, hook mapping, footer layout, startup plan) are pure functions over plain data. Side effects (tmux, git, SQLite, processes) sit at the edges behind small interfaces.
- **Tests where bugs hurt:**
  - Always tested: pure decisions, store queries and migrations, config parsing, protocol encoding.
  - Tested once against a real process: tmux and git, on throwaway servers and repos.
  - Not tested line by line: glue code and simple views. A few key screens get golden snapshots.
- **Agent-friendly, briefly:**
  - One root `AGENTS.md` (about one page): the v1/v2 split in its first lines, the architecture map, dependency rules, build and test commands, conventions.
  - Section-level `AGENTS.md` files (for example `internal/v2/ui/`) only where the code doesn't make the rules obvious. Each has the same shape (purpose, entry points, rules, how to test) and stays under about 40 lines.
  - Package doc comments (`doc.go`) say what a package does. `CLAUDE.md` imports `AGENTS.md`, so every agent reads the same text.

### Draft layout

Imports only point downwards.

```text
internal/v2/
  cli/               thin CLI commands: parse flags, call a service or RPC, print
  app/               composition root: the only place that wires implementations together
  daemon/            process lifecycle, socket server, routing requests to services
  rpc/               request/response types + client, shared by CLI, UI and daemon
  tracks/            use cases: create, end, promote, resume, archive
  track/             domain: Track, Status, Kind, lifecycle transitions (pure, imports nothing internal)
  store/             SQLite: schema, migrations, queries
  agents/            Provider interface; claude/ and cursor/: spawn command, hook install, event mapping
  hooks/             `tracks hook`: payload parsing into domain events
  supervise/         per-track liveness and fallback checks
  tmux/              client with socket, generated config, tmuxtest helper
  footer/            footer layout rendered as a tmux format string (pure)
  workspace/         worktrees and provisioning
  devservers/        dev servers, proxy, ports
  config/ platform/  v2 config schema; paths, profile, shell and log helpers
  ui/
    theme/           palette and Huh theme
    widget/          shared UI pieces, only once 2+ screens use them
    source/          Source interface for UI data; demo and daemon implementations
    mainview/        the Main window (not `main`: reserved in Go)
    menu/ switcher/  popups
  demo/              fake tracks and the fake agent for the playground
```

**Dependency rules:**
- `track` imports nothing internal.
- `ui` reaches data only through `ui/source` and `rpc`.
- Only `app` wires concrete implementations.

**Reused from v1, unchanged at first:** `git`, `provision`, `services`, `proxy`, `ports`, `github`, `notify`, `update`, `usage`, `shellx`, `dlog`.

## Open questions

- **Main:** what Enter does on a track (open an action panel or switch to its window), and where details are shown. To be decided in chunk 3 or 7, informed by the playground.
- **v1 data:** fresh start, or a read-only import into History at release.
- **Final paths at release:** keep the `-v2` names or take over the plain ones.
- **Hover in the footer:** is it worth a Bubble Tea footer pane per window? This is decided after trying the tmux footer in chunk 2.
- **The v1 menu rebuild** (uncommitted, Charm v1, on the `tracks/09ad3c-menu` branch): ship it to v1 too, or keep it only as the reference for the v2 menu.

## Risks

- **Nested tmux:** users who start Tracks inside their own tmux are refused and have to open a new terminal tab. This is a deliberate trade-off.
- **Alt shortcuts on macOS** need the terminal to send Option as Alt (iTerm2 "Esc+", Ghostty `macos-option-as-alt`). The docs must say so.
- **Server environment:** the Tracks tmux server inherits PATH and other variables from the terminal that starts it first.
- **The generated tmux config is opinionated.** Overrides go only through a sourced override file.
- **Agent hook payloads drift** between CLI versions. Hooks read only named fields and fall back to polling.

## Existing work

- [PR #105](https://github.com/bluegardenproject/tracks/pull/105) adds a shared theme and palette to v1 (Charm v1). v2 reuses the palette values, ported to Lip Gloss v2.
- The menu rebuild on the `tracks/09ad3c-menu` branch (Charm v1, uncommitted) is the design reference for the v2 menu.
