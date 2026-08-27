# Design: Cursor CLI integration

**Status:** Research verified against the real CLI and codebase — ready to start.
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

4. ~~**Usage v2 timeline**~~ — **decided 2026-08-27: defer.** Blank cost and
   token columns on Cursor tracks are acceptable; model choice is the point of
   the integration. Note this decides *display* only — the computation still
   has to be gated, or wrong numbers appear by default rather than blanks
   (§8.3).

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

## 7. Verification pass (2026-08-27)

Everything in §2–§3 was re-checked against the installed binary
(`agent` 2026.08.11-e8db854, logged in) and the 1.0.0 codebase rather
than taken from the help text. Findings that change the plan:

| Claim | Verdict | Detail |
|---|---|---|
| `agent create-chat` returns a chat id | ⚠️ **conditional** | Returns a UUID instantly **only with stdin closed**. Bare `agent create-chat` hung with no output and had to be killed at 3 min. |
| `--list-models` can populate the picker | ✅ verified | Emits `id - Label` per line, one per model, with `auto` marked `(current, default)`. Directly parseable. |
| Cursor uses the same model ids as the Claude API | ❌ **false** | They are Cursor-specific: `claude-opus-5-thinking-high`, `gpt-5.3-codex`, `composer-2.5`, `cursor-grok-4.6-low`. See §8.3. |
| Status/idle detection needs work for Cursor | ❌ **not needed** | `nextLiveStatus` is driven by `observePane` diffing a `capture-pane` snapshot (`supervisor.go`), never by the transcript. Running/Waiting works for any binary in the pane, unchanged. |
| `TRACKS_ID` env injection must be dropped | ❌ **false** | See §8.1. |
| Spawn is a wide seam | ✅ narrow | Exactly two call sites: `supervisor.go:103` (`BuildOptions`) and `:123` (`BuildResumeOptions`). |

## 8. Corrections to §3

### 8.1 Keep the env injection (§3b item 4 is wrong)

§3b says "No `TRACKS_ID` env injection: Cursor doesn't read `CLAUDE.md`,
so the env vars are not picked up." That conflates two mechanisms.
`TRACKS_ID` and `TRACKS_SOCKET_DIR` are exported by the shell wrapper
and inherited by *every* process in the pane, whatever the agent binary
is. They are what the in-worktree helper scripts (`tracks-add-repo`)
read — not something Claude parses.

Dropping them would break those helpers inside Cursor tracks for no
reason. Keep the env prefix exactly as-is; what changes is only where
the *instructions* live (`CLAUDE.md` → `.cursor/rules/tracks.mdc`).

### 8.2 `create-chat` must run with stdin closed

The daemon calls this during provisioning. With an inherited terminal
stdin it blocks forever, which turns a provisioning step into a hang
with no error and no timeout — strictly worse than a failure. Set
`cmd.Stdin = nil` and give the command a context deadline; treat a
timeout as a provisioning error carrying the reason.

### 8.3 Cost must be gated on Provider *before* the price lookup

§3d proposes a `Provider` check "in the dashboard renderer". That is too
late. `internal/usage.priceFor` matches model ids by substring, and
Cursor's Anthropic-branded names match it:

| Cursor model | `priceFor` returns | Reality |
|---|---|---|
| `claude-opus-5-thinking-high` | $5 / $25 per MTok | Cursor bills subscription/credits |
| `claude-sonnet-5-thinking-xhigh` | $2 / $10 per MTok | ” |
| `claude-fable-5-thinking-high` | $10 / $50 per MTok | ” |
| `gpt-5.3-codex` | $0 / $0 | ” |
| `composer-2.5`, `auto` | $0 / $0 | ” |

So a Cursor track would show a *mix* of confident-but-wrong dollar
figures and zeroes, which is worse than a uniform blank. Gate at the
point of computation (`addMessage` / wherever a Cursor track's usage is
assembled) so no Anthropic rate is ever applied to a Cursor model, and
render `—` in the COST and TOKENS columns.

Note this is the same class of bug the 1.0 pricing fix addressed —
substring matching being confidently wrong — reappearing through a new
door.

## 9. Implementation plan

Six phases, each independently shippable and reviewable. Phases 1–2 get
a Cursor track running; 3–6 make it pleasant.

**Priority (2026-08-27):** the point of the integration is reaching
models Claude Code can't — GPT-5.3 Codex, Gemini, Grok, Composer — from
inside a track. Phases 1, 2, 4 and 5 are therefore the deliverable;
cost reporting is not, and Phase 6 exists mainly to stop wrong numbers
appearing (§8.3). If effort has to be cut, cut Phase 3's polish before
anything that touches model selection.

### Phase 1 — Provider field through the three data layers (~0.5 d)

- `internal/config`: add a `Cursor` struct mirroring `Claude` (`Binary`
  default `agent`, `Model`, `ModelByKind`, `ModelChoices`). Add
  `Provider` to the top level or per-kind defaults — decide with §10.1.
- `internal/state`: `Track.Provider` and `DraftSpec.Provider`, both
  `omitempty`, empty meaning `claude`.
  **No schema bump** — same reasoning as `RequestedModel`: a v6 store is
  refused outright by a v5 binary, and an absent provider has a correct
  default. Document it at the field.
- `internal/daemon/protocol.go`: `NewParams.Provider`.
- A `state.Provider` string type with `ProviderClaude` / `ProviderCursor`
  constants and an `IsValid`; validate in `Config.Validate` the way
  `model_by_kind` keys now are.

**Done when:** a track can be created with `provider: cursor` recorded
and round-tripped through the store and a draft relaunch, with existing
records still decoding as `claude`.

### Phase 2 — `internal/cursor/spawn.go` (~1 d)

Mirror `internal/claude/spawn.go`, exposing the same two constructors so
the dispatch in Phase 4 is trivial:

```go
func BuildOptions(cfg config.Config, t state.Track, socketDir, sentinelPath string) (SpawnOptions, error)
func BuildResumeOptions(...) (SpawnOptions, error)
```

Differences from the Claude builder:
- `--force` replaces `--permission-mode auto`; `--mode plan` for
  ask/plan kinds (Cursor's read-only modes map onto the worktree-less
  kinds cleanly).
- `--workspace <primary worktree>`, `--add-dir` for the rest.
- `--model` only when non-empty, exactly as the Claude builder does.
- Keep the `TRACKS_ID` / `TRACKS_SOCKET_DIR` prefix and the sentinel
  `touch` + `exec $SHELL -l` tail unchanged (§8.1).
- Resume passes `--resume <chatId>` and, as with Claude, **no**
  `--model`.

Session pre-creation belongs here as `CreateChat(ctx, binary) (string, error)`:
`exec.CommandContext`, `Stdin = nil`, deadline, trim the UUID, error on
anything that doesn't parse as one.

**Tests:** the shell-command assertions mirror `internal/claude/model_test.go`
— flag present/absent, resume never carries `--model`, no `--model ''`
when unset, env prefix intact. `CreateChat` gets a fake-binary test
(a script that echoes a UUID, one that hangs → deadline error).

### Phase 3 — Context injection (~0.5 d)

Write `<primary worktree>/.cursor/rules/tracks.mdc` during provisioning
with the task suffix Claude gets via `taskSuffix`, plus the literal
`TRACKS_ID` / socket dir. Remove at teardown.

`provision.Run`/`provisionOptions` (`handlers.go:509`, `:761`) is the
seam. Gate on provider so Claude tracks don't grow a `.cursor/`
directory.

The pre-push review gate currently lives in `taskSuffix` and is
mandatory for Claude tracks; it must be reproduced here or Cursor tracks
silently lose it. That is a behaviour difference worth calling out in
the PR, not a footnote.

### Phase 4 — Supervisor dispatch (~0.5 d)

Replace the two direct calls with a provider switch:

```go
switch t.Provider {
case state.ProviderCursor: opts, err = cursor.BuildOptions(...)
default:                   opts, err = claude.BuildOptions(...)
}
```

Extract a tiny `spawnOptionsFor(t)` helper so both call sites and any
future provider share one dispatch point. `default` (not an explicit
`claude` case) keeps old records working.

### Phase 5 — TUI provider + model picker (~1 d)

- Provider select ahead of the model field in all three flows
  (`Run`, `runReview`, `runDocReview`).
- The model list comes from the chosen provider: config choices for
  Claude, `agent --list-models` for Cursor, parsed as `id - Label`,
  cached per TUI session and falling back to a bare text input if the
  call fails or the binary is missing.
- `TestEveryFlowWithAModelPickerSendsIt` already guards the "picker
  shown but answer discarded" bug for `Model`; extend it to `Provider`
  in the same pass, or the same class of bug ships again.

### Phase 6 — Usage stub + dashboard (~0.5 d)

Cost display is explicitly out of scope (decided 2026-08-27, §5 q4).
That makes this phase small, but **not empty**: the danger was never a
missing number, it was a wrong one. Leaving the computation ungated
produces Anthropic rates against Cursor model names (§8.3), so the gate
ships even though the feature doesn't.

- Skip transcript parsing entirely for Cursor tracks.
- Gate cost at computation (§8.3), render `—` for COST and TOKENS.
- MODEL keeps working: it shows `RequestedModel` for Cursor, since
  `ObservedModel` is only ever derived from a Claude transcript. Worth a
  comment at the renderer — otherwise the next reader "fixes" the
  inconsistency.

**Total: ~4 d**, matching §4. Phases 1, 2 and 4 are the critical path to
a first working Cursor track.

## 10. Decisions still needed

1. ~~**Is provider per-track only, or also per-kind?**~~ — **decided
   2026-08-27: per-track only.** Settings carries a single default
   provider; the creation form pre-selects it and the user can override
   per track. No `provider_by_kind` — `model_by_kind` stays the only
   per-kind dimension.
2. ~~**`.cursor/` containment**~~ — **dissolved 2026-08-27.** Nothing is
   written into the worktree at all; see §11.
3. **Auth** (§5 q2). `agent status` confirms a logged-in user here, so
   the zero-config path already works; `cursor.api_key` in tracks config
   would be the only reason to add secret handling to a file that
   currently holds none. Recommend delegating to the user's environment
   and documenting it.
4. **Review gate parity** (new). Claude tracks get a mandatory pre-push
   review via `taskSuffix`. Decide whether Cursor tracks get the same
   text in `tracks.mdc` or deliberately run without it.

## 11. Context injection, revised — a global rule, not a per-worktree file

§3c proposed writing `<worktree>/.cursor/rules/tracks.mdc` at provision
time and deleting it at teardown. Testing showed that is unnecessary.

**Measured 2026-08-27** with a probe rule containing a codeword, asking
`agent -p` to repeat it:

| Rule location | Run from | Picked up? |
|---|---|---|
| `~/.cursor/rules/` | a real workspace | ✅ yes |
| `<workspace>/.cursor/rules/` | same workspace | ✅ yes |
| `~/.cursor/rules/` | a non-workspace dir (`$TMPDIR`) | ❌ no |

So the CLI honours **global user rules**, and the one negative was an
artifact of running outside a workspace — every tracks pane runs inside
one, so it does not apply.

That means Cursor context installs exactly the way Claude's already
does. `InstallGlobalHelpers` (`internal/daemon/skill.go`, called from
`server.go` at daemon start) writes `~/.claude/agents/tracks-reviewer.md`
and friends from templates embedded in the binary; it grows one more
write:

```
~/.cursor/rules/tracks.mdc     ← new, alwaysApply: true
```

Format, taken from rules already on disk:

```markdown
---
description: <one line>
alwaysApply: true
---
```

**The one constraint this imposes.** A global rule loads in *every*
Cursor session the user runs, including their ordinary work outside
tracks. So it must be conditional from its first line — "if the
`TRACKS_ID` environment variable is set you are inside a tracks
worktree; otherwise ignore this rule" — and it must stay short, because
its cost is paid on every unrelated session too. Per-track values are
read from the environment (§8.1) rather than baked in, which is what
makes one static template sufficient.

**What this removes from the plan:** Phase 3 is no longer "write at
provision, delete at teardown, keep it out of git". It is one more
template and one more `os.WriteFile` in a function that already does
three. No worktree writes, no `.gitignore` interaction, no teardown
path, and no risk of a stray `.cursor/` being committed.

**What it does not solve.** `~/.cursor/agents/` exists but is empty and
has no documented user-definable format; `cli-config.json` mentions an
internal `exploreSubagentModel`, not a user surface. There is still no
Cursor equivalent of `tracks-reviewer`, so the mandatory pre-push review
(§10 item 4) remains genuinely open — it is the one part of the Claude
context that cannot be ported by writing a file.

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
