# Design: Cursor CLI integration

**Status:** Research complete — not yet started.
**Last updated:** 2026-08-27

Goal: let users choose **Cursor Agent** (`agent`) as the AI provider when
creating a track, alongside the existing Claude Code (`claude`) provider. Tracks
manages the worktree, session, and tmux lifecycle identically for both; the
provider controls what binary runs and how context is injected.

---

## 1. Why Cursor

Cursor Agent (`cursor agent` / bare `agent`) is a terminal-native autonomous
coding agent — same headless model as Claude Code, but:

- **Multi-provider models**: surfaces GPT-5, Gemini, Grok, and Anthropic models
  from a single binary. No need to switch tools to access different frontier
  models.
- **Independent quality signal**: useful for cross-checking results or for work
  where a non-Claude model is preferred.
- **`--yolo` mode**: equivalent to `--permission-mode auto` — full write + shell
  access, no prompts.

---

## 2. Flag alignment with Claude Code

The CLI interfaces are close enough that `internal/cursor/spawn.go` is nearly a
find-replace of `internal/claude/spawn.go`.

| Tracks concept          | Claude flag                    | Cursor flag              | Delta        |
|-------------------------|--------------------------------|--------------------------|--------------|
| Task prompt             | positional arg                 | positional arg           | ✅ identical  |
| Non-interactive mode    | `--print`                      | `--print`                | ✅ identical  |
| Extra workspace dir     | `--add-dir <path>`             | `--add-dir <path>`       | ✅ identical  |
| Model selection         | `--model <model>`              | `--model <model>`        | ✅ identical  |
| Permission mode         | `--permission-mode auto`       | `--force`                | rename only  |
| Session pre-creation    | `--session-id <uuid>`          | `agent create-chat` → id | small adapter|
| Session resume          | `--resume <sessionID>`         | `--resume <chatId>`      | ✅ identical  |
| Output format           | n/a                            | `--output-format json`   | Cursor extra |
| Execution mode          | n/a                            | `--mode plan\|ask`       | Cursor extra |

Full `agent --help` output is preserved in the Appendix.

---

## 3. What needs building

### 3a. Provider field in config / state / protocol

Extend the three data layers with a `provider` field alongside `model` (which
the 1.0 model-picker work already added):

- `config.go` — add a `Cursor` struct (like the existing `Claude` struct):
  ```yaml
  cursor:
    binary: agent          # default; override if installed elsewhere
    model: ""              # global default model for new Cursor tracks
  ```
- `protocol.go` `NewParams` — add `Provider string` (values: `"claude"`,
  `"cursor"`; default `"claude"` to keep existing behaviour).
- `state.go` `Track` / `DraftSpec` — add `Provider string` alongside the
  existing `Model string`.

### 3b. `internal/cursor/spawn.go`

Mirror of `internal/claude/spawn.go`. Key differences:

1. **Session pre-creation**: before the first spawn, run `agent create-chat`
   and capture the returned chatId. Store it as `Track.SessionID`. Pass
   `--resume <chatId>` from run 0 onwards (Cursor expects `--resume` even on
   the first real run after `create-chat`).

2. **Permission flag**: `--force` instead of `--permission-mode auto`.

3. **Working directory**: use `--workspace <cwd>` instead of relying on the
   tmux pane's cwd (more explicit, same outcome).

4. **No `TRACKS_ID` env injection**: Cursor doesn't read `CLAUDE.md`, so the
   env vars are not picked up. Replaced by context injection (§3c).

Resulting command shape:
```
CURSOR_API_KEY=… sh -c '
    agent <prompt> --force --resume <chatId> \
        --workspace <primaryWorktree> \
        [--add-dir <otherWorktree>]* \
        [--model <model>]
    touch <sentinelPath>
    exec ${SHELL:-bash} -l
'
```

### 3c. Context injection — `.cursor/rules/tracks.mdc`

Claude reads `CLAUDE.md` files from `$TRACKS_ID` / `$TRACKS_SOCKET_DIR` env
vars. Cursor reads rules from `.cursor/rules/*.mdc` files in workspace roots.

At provision time (in `provision.Setup()`), write a file into the primary
worktree:

```
<primaryWorktree>/.cursor/rules/tracks.mdc
```

Content: the Tracks system prompt with `TRACKS_ID` and `TRACKS_SOCKET_DIR`
baked in as literals for that track. Delete the file during worktree teardown.

For multi-repo tracks, write into the primary worktree only — Cursor loads
rules from all `--add-dir` roots, so the file will be found.

The `.cursor/` directory should be added to `.gitignore` (or written to the
worktree only, not the branch) so it doesn't leak into commits.

### 3d. Usage / transcript parsing

Cursor writes no transcript file. The current `internal/usage/` package reads
Claude's `~/.config/claude/projects/**/<sessionID>.jsonl`.

**Recommended approach (v1): stub it.**

- `Model` is explicit at spawn time (from `--model` flag or config default), so
  the MODEL column in the dashboard works without any parsing.
- Token and cost columns show `—` for Cursor tracks (set a sentinel zero value
  + a `Provider` check in the dashboard renderer).
- Revisit in v2: Cursor's `--output-format stream-json --print` likely includes
  usage fields; tee it to a per-track `.jsonl` log and parse on the same
  refresh cadence as the Claude transcript reader.

### 3e. TUI — provider + model picker in `newtrack`

Add a **Provider** picker before (or as the first item of) the existing model
picker step. Provider selection narrows the model list:

```
Provider: [ Claude Code ▾ | Cursor Agent ]
Model:    [ auto | claude-sonnet-4-6 | … ]   (Claude list)
          [ auto | gpt-5 | gemini-3 | … ]    (Cursor list, from agent --list-models)
```

`agent --list-models` can populate the Cursor list at TUI open time (cache the
result). The `"auto"` option omits `--model` and lets the binary use its own
default.

---

## 4. Effort estimate

Assumes the 1.0 model-picker work (which adds `Model` to `NewParams`, `Track`,
config, and the TUI) is already merged.

| Phase | Work | Est. |
|---|---|---|
| Provider field in config / state / protocol | Mechanical extension of model-picker changes | 0.5 d |
| `internal/cursor/spawn.go` + `create-chat` pre-creation | Near-clone of Claude spawn | 1 d |
| `.cursor/rules` context injection in provisioner | New file write at setup, delete at teardown | 0.5 d |
| Supervisor dispatch (provider switch in daemon) | `switch track.Provider { … }` | 0.5 d |
| TUI provider + model picker | Extend existing model picker step | 1 d |
| Usage stub + dashboard `—` for Cursor | Guard on `track.Provider` | 0.5 d |
| **Total** | | **~4 d** |

---

## 5. Open questions

1. **`.cursor/` in worktree**: should the rules file be written outside the git
   tree (e.g. a tempdir symlinked in) to avoid any risk of accidental commits,
   or is writing to the worktree + `.gitignore` entry sufficient?

2. **`CURSOR_API_KEY` management**: Cursor auth is either `agent login` (stores
   to `~/.cursor/`) or `CURSOR_API_KEY` env var. The env-var path is cleanest
   for Tracks (no side effects on the user's global auth state). Where should
   the key live — in `~/.config/tracks/config.yaml` under `cursor.api_key`, or
   delegated entirely to the user's shell env?

3. **Multi-repo context**: Cursor loads rules from all `--add-dir` roots. If
   each repo has its own `.cursor/rules/` in the real checkout, those rules
   will also load. Is that desirable, or should the tracks.mdc file be the only
   rule source for Tracks-managed sessions?

4. **Usage v2 timeline**: defer until there's demand, or implement alongside v1
   to avoid a visible regression (missing cost data) compared to Claude tracks?

---

## 6. Sequencing

This work can start immediately after 1.0 is stable. No other open topic blocks
it. Suggested order:

1. Provider + model field wiring (config / state / protocol)
2. `internal/cursor/spawn.go` — get a Cursor track running end-to-end
3. Context injection (`.cursor/rules`)
4. TUI picker
5. Dashboard + usage stub

---

## Appendix: `agent --help` output (2026-08-27)

```
Usage: agent [options] [command] [prompt...]

Start the Cursor Agent

Arguments:
  prompt                       Initial prompt for the agent

Options:
  -v, --version                Output the version number
  --api-key <key>              API key for authentication (can also use
                               CURSOR_API_KEY env var)
  -H, --header <header>        Add custom header to agent requests
  -e, --endpoint <url>         Target API endpoint URL (default:
                               "https://api2.cursor.sh")
  -p, --print                  Print responses to console (for scripts or
                               non-interactive use). Has access to all tools,
                               including write and shell. (default: false)
  --output-format <format>     Output format (only works with --print):
                               text | json | stream-json (default: "text")
  --stream-partial-output      Stream partial output (only with --print and
                               stream-json) (default: false)
  --mode <mode>                Start in the given execution mode. plan:
                               read-only/planning. ask: Q&A style, read-only.
                               (choices: "plan", "ask")
  --plan                       Start in plan mode (shorthand for --mode=plan)
  --resume [chatId]            Select a session to resume (default: false)
  --continue                   Continue previous session (default: false)
  --model <model>              Model to use (e.g., gpt-5, sonnet-4-thinking).
                               Parameterized: 'claude-opus-4-8[context=1m,effort=high]'
  --list-models                List available models and exit
  -f, --force                  Force allow commands unless explicitly denied
  --yolo                       Alias for --force (default: false)
  --auto-review                Smart Auto: server classifier auto-runs safe
                               tool calls and prompts for the rest
  --sandbox <mode>             Enable or disable sandbox mode
                               (choices: "enabled", "disabled")
  --approve-mcps               Automatically approve all MCP servers
  --trust                      Trust the current workspace without prompting
  --workspace <path-or-name>   Workspace directory (defaults to cwd)
  --add-dir <path>             Add an additional workspace root directory
                               (can be specified multiple times)
  --plugin-dir <path>          Load a local plugin directory
  -w, --worktree [name]        Start in an isolated git worktree
  --worktree-base <branch>     Branch or ref to base the new worktree on
  --skip-worktree-setup        Skip worktree setup scripts from
                               .cursor/worktrees.json
  -h, --help                   Display help for command

Commands:
  install-shell-integration    Install shell integration to ~/.zshrc
  uninstall-shell-integration  Remove shell integration from ~/.zshrc
  login                        Authenticate with Cursor
  logout                       Sign out and clear stored authentication
  mcp                          Manage MCP servers
  plugin                       Manage plugins and plugin marketplaces
  worker [options]             Start a private cloud worker
  status|whoami [options]      View authentication status
  models                       List available models for this account
  bedrock                      Configure AWS Bedrock usage for CLI
  about [options]              Display version, system, and account information
  update                       Update Cursor Agent to the latest version
  create-chat                  Create a new empty chat and return its ID
  generate-rule|rule           Generate a new Cursor rule with interactive prompts
  agent [prompt...]            Start the Cursor Agent
  ls                           Resume a chat session
  resume                       Resume the latest chat session
  help [command]               Display help for command
```
