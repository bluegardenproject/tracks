# tracks

Run multiple [Claude Code](https://docs.claude.com/en/docs/claude-code) agents
in parallel, in isolated git worktrees, coordinated from a single tmux
session — without ever moving the branch of the checkout your editor
(Cursor, VS Code, …) is watching.

`tracks` is a small Go CLI that lives alongside `stac-man` and
`github-butler` and mirrors their conventions.

## Why

Claude Code's built-in `claude -w` flag creates worktrees by reusing the
primary checkout's HEAD, which yanks the editor onto a different branch.
`tracks` keeps the primary checkout invariant by:

1. Creating a fresh `<type>/<slug>` branch in a dedicated worktree under
   `~/.local/state/tracks/worktrees/<track-id>/<repo>/`.
2. Running Claude there with `--add-dir` for every selected repo.
3. Removing the worktree when the track ends — **the branch survives
   locally**, so you can `git checkout <type>/<slug>` from the primary
   checkout afterwards.

A long-running daemon owns the tmux session and one supervisor per track.
The daemon dies when the tmux session is killed and survives detach.

## Install

```bash
make install   # → ~/bin/tracks
```

Requires Go 1.25 + `git` + `tmux` + the `claude` CLI on `PATH`.

## Configure

`~/.config/tracks/config.yaml`:

```yaml
schema_version: 1

tmux:
  session_name: tracks

claude:
  binary: claude
  permission_mode: auto

branch:
  types: [feat, fix, chore, refactor, docs, test]
  default_type: fix

repos:
  - name: ledger-live
    path: ~/ledger/ledger-live
    base: develop
    init_submodules: true        # off by default; opt-in for repos with submodules
  - name: swap-live-app
    path: ~/ledger/swap-live-app
    base: develop
  - name: lumen
    path: ~/ledger/lumen
    base: main
```

Anything you omit picks up sensible defaults.

## First track

```bash
# Start (or attach to) the tmux session and daemon.
tracks

# In any terminal:
tracks new
# → multi-select repos (space toggles, enter confirms)
# → pick branch type (feat / fix / chore / …)
# → enter a slug (lowercase, digits, hyphens)
# → type the task
# → daemon creates worktrees + spawns Claude
# → a tmux window `t-<id>` opens with the live filtered log

# See everything at a glance:
tracks dashboard

# Force-end a track (worktree gone, branch kept):
tracks done <track-id>
tracks kill <track-id>   # SIGKILL variant

# Re-attach to a track's window (recreates it if you closed it):
tracks attach <track-id>

# Clean up orphan worktrees (after a crash):
tracks gc

# When done for the day:
tmux kill-session -t tracks   # also stops the daemon
```

## Cross-repo work

`tracks new` already mounts multiple repos in one track via `--add-dir`.
If Claude discovers mid-task that it needs another repo, it has access to
the `tracks-add-repo` skill (installed automatically into each worktree):

```bash
tracks add-repo <repo-name>
```

The daemon provisions a fresh worktree on the same branch and returns
its absolute path. Claude continues without restart.

## Commands

| Command                | Purpose                                                |
| ---------------------- | ------------------------------------------------------ |
| `tracks`               | Bootstrap the tmux session + daemon, then attach.      |
| `tracks new`           | Interactive picker → daemon creates a new track.       |
| `tracks ls`            | Tabular list of every known track.                     |
| `tracks dashboard`     | Live bubbletea TUI; approve pending prompts; switch.   |
| `tracks attach <id>`   | Switch the tmux client to a track's window.            |
| `tracks log <id>`      | Filtered tail of a track's stream-json log.            |
| `tracks done <id>`     | Graceful end: SIGTERM Claude, remove worktrees.        |
| `tracks kill <id>`     | Forceful end: SIGKILL Claude, remove worktrees.        |
| `tracks add-repo <r>`  | (Run from inside a track) mount another repo.          |
| `tracks gc`            | Remove orphan worktrees. `--branches` prunes empties.  |
| `tracks ping`          | Health-check the daemon.                               |
| `tracks version`       | Print binary version + build time.                     |

## Architecture summary

```
~/ledger/tracks/             ← source
~/bin/tracks                 ← installed binary
~/.config/tracks/config.yaml ← user prefs
~/.local/state/tracks/
  ├── state.json             ← daemon state (schema-versioned, atomic writes)
  ├── logs/<id>.jsonl        ← Claude stream-json per track
  └── worktrees/<id>/<repo>/ ← one worktree per repo per track
$XDG_RUNTIME_DIR/tracks/sock ← daemon socket  (or $TMPDIR/tracks-<uid>/sock)
```

The daemon is a tmux-server child (spawned via `tmux run-shell -b`). It
polls `tmux has-session` every 2 seconds and exits cleanly when the
session is killed.

## Safety invariant

`tracks` **never** mutates the primary checkout. It only:

- runs `git fetch <remote> <base>` (writes to `.git/refs/remotes/`),
- runs `git worktree add/remove/list/prune` against the primary `.git`.

Neither of those touches the primary's HEAD or working tree. The
`internal/git.PrimaryRepoClient` type enforces this by construction —
no method exists for anything else.

## V2 / known gaps

- **Permission-prompt MCP bridge.** The daemon has socket support for
  pending prompts; the dashboard has the approve/deny UI; but plumbing
  Claude's `--permission-prompt-tool` into the daemon socket requires
  an MCP server. `tracks` v1 relies on `--permission-mode auto`
  instead. If Claude pauses for approval, attach to its window
  (`tracks attach <id>`) and answer there.
- **Console REPL window.** Currently the `console` tmux window is an
  ordinary shell. The plan's `tracks console` REPL with tab completion
  is deferred.
- **Cross-platform.** `internal/daemon/lock.go` uses POSIX flock; this
  is macOS / Linux only.

## Layout

```
cmd/                  cobra subcommands; each file registers itself via init()
internal/
  config/             ~/.config/tracks/config.yaml (XDG-aware, atomic save)
  state/              schema-versioned state.json, Store interface + FileStore + MemoryStore
  git/                PrimaryRepoClient (limited surface) + WorktreeClient
  daemon/             socket server, supervisors, recovery, skill install
  claude/             Spawn + stream-json TailLog (event registry)
  tmux/               session + window operations
  tui/
    newtrack/         charmbracelet/huh picker flow
    dashboard/        bubbletea live dashboard
scripts/
  spike-worktree.sh   throwaway: validates the Cursor isolation invariant
```

Mirrors `stac-man` / `github-butler`.
