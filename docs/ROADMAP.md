# Tracks roadmap

What's open after v2.0.0. Add freely; delete an item once it ships. v1's
design notes (`docs/design/`) and the v2 plans (`docs/v2/`) were deleted with
the v2.0.0 release; git history keeps them.

## Next

- **Backends and repo config** ([docs/design/backends.md](design/backends.md)):
  a checked-in `.tracks/config.yaml`, named ports, proxy groups, Scala, Python
  and TypeScript services next to frontends. Steps 0–9, one PR each.
- **Reviews** ([docs/design/review.md](design/review.md)): steps 1–9 are
  built: a reviewer on any engine and model in a train's session, a review packet
  built by Tracks, code-owner scope, likelihood and confidence per finding.
  Open: the verify pass (step 10) and calibration (step 11).
- **Trains** ([docs/design/trains.md](design/trains.md)): read-only agents a
  track hands tasks to, on the track's Platform, talking through Tracks'
  relay. Steps 1–8 (the Platform, the overlay, Collect messages), 9a, 10
  and 11 are built, 9b in review; v1 has the Reviewer as its only train.
  Phase 2: side tracks, child
  tracks a main track orchestrates to write code in parallel, on its
  relay, grouped under it in the Station.
- **Agent protocol (v2)** ([docs/design/protocol.md](design/protocol.md)):
  exploring. Plugins (the Investigator and the Watcher as the first,
  maybe in a repo of plugins), a manifest, a versioned message envelope,
  Tracks as an MCP server for its tools, and programmable messages that
  Tracks runs with the user as the final say. Needs planning first.
- **GitHub on the relay** (in [trains](design/trains.md), Phase 2): poll
  the notifications endpoint and one GraphQL query instead of each PR,
  and post `@github` events (merged, conflicts, CI failed, new comments)
  that wake the track's agent to act on them.
- **Worktree setup:** submodules (step 4 of Backends).
- **Homebrew:** a formula built from the release binaries and checked against
  `SHA256SUMS`, updated by each release, depending on tmux. A brew-installed
  Tracks leaves updates to `brew upgrade`: `tracks update` and the update check
  say so instead of replacing the binary. Open: an own tap or homebrew-core.
- **Confirm Cursor's markers on a real screen:** the plan-approval and
  mode-switch markers Tracks reads from the Cursor agent CLI.

## Follow-ups

- Drop the Station's Cost column: it counts Claude only, and inaccurately;
  the engines' own apps and dashboards show what was spent.

- Renaming a repo that tracks use.
- The model a track actually ran, including what Cursor's "auto" picked.
- The folder trust prompt for Ask and Plan tracks without repos.
- Cursor `create-chat` hangs with an unused `XDG_CONFIG_HOME`.
- Agents asking questions in plain text instead of their question tool.
- `NOCASE` folds only A–Z.
- Proxy and dev servers ([docs/design/proxy.md](design/proxy.md)):
  - A track window closed outside Tracks leaves its dev servers only tmux's
    SIGHUP; nothing cleans up servers orphaned by a crash.
  - Adding a repo with a setup or dev servers to a running track doesn't
    tell its agent, whose prompt has no Setup or Dev servers paragraph.
  - A server whose setup failed shows as crashed in `tracks servers` and the
    Proxy tab.
  - The Proxy tab doesn't dim a guessed server type yet.
  - `ui/tracksview/model.go` and `details.go` are over 400 lines.

## Features

- **Fast Tracks** (the Settings section is a placeholder): templates that preset
  repos, kind, engine and setup commands, stored in the database. Station's
  Fast Track frame has the first one built in, Fast Review: a pull request
  link pasted there opens a Review track on it. How to start the others is
  open.
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
- **Track actions** (the PR comment buttons, moving to the sidebar's
  Signals with [trains](design/trains.md)): more buttons beside "open
  comments", such as failing CI; the prompts they send editable in Settings,
  and a setting for whether review fixes are amended (force-pushed) or added
  as new commits; a key to run them without the mouse. Now and then an
  uplifting message in the row instead of the track's info, behind a setting.
