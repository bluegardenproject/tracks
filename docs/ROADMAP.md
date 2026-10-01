# Tracks roadmap

What's open after v2.0.0. Add freely; delete an item once it ships. v1's
design notes (`docs/design/`) and the v2 plans (`docs/v2/`) were deleted with
the v2.0.0 release; git history keeps them.

## Next

- **Proxy and dev servers, one topic of their own.** `tracks up`, `down`,
  `services` and `url`, a Proxy tab, and the repos' services. v1's proxy UI and
  UX didn't work, so this is redesigned rather than ported; v1's design was
  `docs/design/dev-servers.md`. The dev-server text comes back into the work
  and review prompts with it.
- **Worktree setup:** installing dependencies in a new worktree, copying ignored
  files such as `.env` into it, and submodules.
- **Homebrew:** a formula built from the release binaries and checked against
  `SHA256SUMS`, updated by each release, depending on tmux. A brew-installed
  Tracks leaves updates to `brew upgrade`: `tracks update` and the update check
  say so instead of replacing the binary. Open: an own tap or homebrew-core.
- **Confirm Cursor's markers on a real screen:** the plan-approval and
  mode-switch markers Tracks reads from the Cursor agent CLI.

## Follow-ups

- Renaming a repo that tracks use.
- The model a track actually ran, including what Cursor's "auto" picked.
- The folder trust prompt for Ask and Plan tracks without repos.
- Cursor `create-chat` hangs with an unused `XDG_CONFIG_HOME`.
- Agents asking questions in plain text instead of their question tool.
- `NOCASE` folds only A–Z.

## Features

- **Fast Tracks** (the Settings section is a placeholder): templates that preset
  repos, kind, engine and setup commands, stored in the database. How to start
  one is open.
- **Quick switcher** popup (`Ctrl+b s`), and more Quick Access entries.
- **Each repo's own MCP servers** (`.mcp.json`, `.cursor/mcp.json`) on the
  Engines tab.
- **Prompts:** viewing and editing the prompts each engine gets, with v1's text
  as the default.
- **Notifications:** clicking one switches to its track; platforms other than
  macOS.
- **Status:** a "your turn" status; the running tool and the last message as a
  snippet; CI checks and review state on PRs; hooks talking back (review gates);
  a screen-polling fallback for CLIs without hooks.
- **Supervision:** restarting a failing agent automatically, with a limit;
  telling a crash from a failed start.
- **Resuming an archived track,** behind a setting.
- **An event timeline** per track.
- **History:** search, filters and paging.
- **PR actions** in the track details.
