# tracks

Run multiple [Claude Code](https://docs.claude.com/en/docs/claude-code) agents
in parallel, each in its own git worktree, coordinated from a single tmux
session. Your editor's branch never moves while Claude is working.

## Install

```bash
make install   # → ~/bin/tracks
```

Requires Go 1.25, `git`, `tmux`, and the `claude` CLI on `PATH`.

## Use

```bash
tracks
```

Starts the tmux session, launches the dashboard, brings up the daemon.

Inside the session, press `<prefix>+t` to open the menu:

- **New track** — pick repos → branch type → task prompt. Claude is spawned
  in a fresh worktree on `<type>/<auto-slug>`. The slug is derived from the
  task prompt: if the prompt mentions a Jira-style ticket (e.g. `ABC-123`) it's
  used as the prefix, followed by the first few descriptive words.
- **Dashboard** — live list of all tracks, statuses, PR URLs.
- **List / Attach… / End… / Kill…** — manage tracks.
- **Settings** — add, edit, or remove repos via a guided form (no YAML
  editing).
- **Quit session** — kills tmux and the daemon; running Claudes get SIGTERM.

When a track ends, its worktree is removed but the branch stays locally so
you can `git checkout <branch>` from your editor afterwards.
