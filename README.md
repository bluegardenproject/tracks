# tracks

Run multiple [Claude Code](https://docs.claude.com/en/docs/claude-code) agents
in parallel, each in its own git worktree, coordinated from a single tmux
session. Your editor's branch never moves while Claude is working.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/bluegardenproject/tracks/main/scripts/install.sh | bash
```

Downloads the matching binary from the latest release into `~/.tracks` and
adds it to your `PATH`. Re-run it any time to upgrade — a daemon from the
previous version restarts automatically on the next `tracks` run. Uninstall
with [`scripts/uninstall.sh`](scripts/uninstall.sh).

Requires `git`, `tmux`, and the `claude` CLI on `PATH`. Linux and macOS only.

### From source

```bash
make install   # builds with Go 1.25 → ~/bin/tracks
```

## Use

```bash
tracks
```

Starts the tmux session, launches the dashboard, brings up the daemon.

Inside the session, press `<prefix>+t` to open the menu:

- **New track** — pick a track type, then repos → task prompt. A **Work**
  track spawns Claude in a fresh worktree on `<type>/<auto-slug>`; the slug is
  derived from the task prompt (a Jira-style ticket like `ABC-123` becomes the
  prefix, followed by the first few descriptive words). **Ask** and **Plan**
  are read-only against your primary checkout and can be promoted to a
  worktree later. **Review** checks a PR or branch out detached and diffs it.
  **Doc review** points at a file on disk — a spec, one-pager, or deck
  exported to PDF — and reports findings on it, then offers to save the report
  next to the document. Two sections are switchable, both on by default:
  **Opinion** (is the argument sound, does the content hold up, how easily
  does it read) and **Claim check** (fact-check its claims against your repos,
  GitHub, and Jira). Review and Doc review also ask for a **candor** level
  from 1 (radical candor) to 10 (honest but gently framed), which changes how
  the findings are worded and nothing else.
- **Dashboard** — live list of all tracks, statuses, PR URLs.
- **Reopen interrupted tracks** — brings back everything that was still
  running when tracks was last quit (see below).
- **List / Attach… / End… / Kill…** — manage tracks.
- **Settings** — add, edit, or remove repos via a guided form (no YAML
  editing).
- **Quit session** — kills tmux and the daemon; running Claudes get SIGTERM.

When a track ends, its worktree is removed but the branch stays locally so
you can `git checkout <branch>` from your editor afterwards.

## Quitting and coming back

Quitting tracks doesn't finish your tracks. Anything still live is recorded
as `interrupted` — its branch, worktree and Claude session all survive — and
the next `tracks` offers to bring them back:

```
2 track(s) were still running when tracks last stopped:
  20260805-091305-1e1ec7  swap-rate-tooltip
  20260805-095525-ee2eb3  reopen-tracks

Reopen 2 interrupted track(s)? [Yes / Cancel]
```

Each reopened track gets its worktree back and a fresh window running
`claude --resume`, so the conversation continues where it stopped — Claude
picks up with the full history and waits for your next message. Cancel and
they stay as they are; run `tracks reopen` (or the menu entry) whenever you
want them. Interrupted tracks are never touched by `X` (clear completed) or
`tracks gc`, so their worktrees wait for you.

To be done with one, **close** it (`d` in the dashboard) — that removes the
worktree and keeps the branch. **Removing** it (`x`) drops the dashboard entry
itself; the worktree stays on disk until the next `tracks gc` reclaims it.
Because that throws away the record — task prompt, cost, PR links, and the
handle `tracks reopen` needs — `x` and `X` (remove all completed) ask for a
`y`/`n` confirmation first.

### Track status

| status | meaning |
| --- | --- |
| `running` / `waiting` | Claude is working, or sitting at a question |
| `pr open` / `prs open` | Claude finished and left pull requests open; the track stays alive so review comments and follow-up commits still land in it |
| `pr merged` / `all merged` | every PR the track opened was merged — the work shipped |
| `done` | finished without a merged PR: none was opened, one was closed unmerged, or you closed the track yourself |
| `interrupted` | tracks was quit while this one was live — reopenable |
| `errored` / `draft` | creation or the session failed / saved but never launched |

Tracks that open several PRs get each one polled and listed separately in the
dashboard's detail panel; emit one `TRACKS_PR_URL=<url>` line per PR.
