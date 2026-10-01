# tracks

Run coding agents ([Claude Code](https://docs.claude.com/en/docs/claude-code)
and the [Cursor CLI](https://cursor.com/cli)) in parallel, each in its own git
worktree, from one tmux session. Your editor's checkout never moves while an
agent works.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/bluegardenproject/tracks/main/scripts/install.sh | bash
```

Downloads the binary for your system from the latest release into `~/.tracks`
and adds it to your `PATH`, after checking it against the `SHA256SUMS`
published with the release. If it can't verify the download, it installs
nothing and exits non-zero. Uninstall with
[`scripts/uninstall.sh`](scripts/uninstall.sh).

`tracks update` installs a newer release over the running binary
(`tracks update --check` only reports). The daemon restarts on the next
`tracks`, and tracks still running are interrupted; Resume brings them back.

Requires `git`, tmux 3.2 or newer, and the `claude` or Cursor `agent` CLI on
your `PATH`. Linux and macOS only.

### From source

```bash
make build                            # ./tracks, with Go 1.25
scripts/install.sh --local ./tracks   # into ~/.tracks, with the PATH setup
```

`make install` copies the build to `~/.tracks` directly.

## Use

```bash
tracks
```

Opens Tracks on its own tmux server, with the Tracks window first. Start it
from a plain terminal: inside another tmux it refuses rather than nesting.
Your `~/.tmux.conf` isn't used; personal tmux overrides go in
`~/.config/tracks/tmux.conf`.

The **Tracks window** has four tabs:

- **Station:** your tracks with their status and PRs. Select one for its
  details and actions: Resume, Archive, Derail, Promote.
- **Repositories:** the repos tracks can work in, each with its base branch.
- **Engines:** the agent CLIs Tracks found, their versions and models.
- **Settings:** theme and theme creator, defaults per track type, archiving,
  notifications, keys.

Every track has a window of its own: the agent on the left, terminals on the
right. The footer on every window lists the tracks; click one, or use the keys
below.

| Keys | |
|---|---|
| `Ctrl+b q` | Quick Access: New track, Tracks filter, Close Tracks |
| `Ctrl+b n` / `p` | next / previous track |
| `Ctrl+b <` / `>` | first / last track |
| `Ctrl+b 1`…`9` | track by number |
| `Ctrl+b t` | add a terminal to the track |

### Track types

- **Work:** a fresh worktree per repo, on a branch of its own.
- **Ask** and **Plan:** read-only in your checkout. Promote one to make it a
  Work track with its own worktrees.
- **Review:** checks a PR or branch out and reviews it.
- **Doc review:** reviews a file on disk (a spec, a one-pager, a deck as PDF),
  optionally with an opinion and a check of its claims against your repos.

Review and Doc review take a **candor** level from 1 (radical candor) to 10
(honest, gently framed), which changes only how findings are worded.

### Inside a track

The agent can run these, and so can you in a track's terminal:

- `tracks review` reviews the branch with a separate agent session.
- `tracks terminal` opens a terminal pane beside the agent.
- `tracks add-repo <repo>` adds a worktree of another repo to the track.
- `tracks promote` turns an Ask or Plan track into a Work track.

Agents announce a pull request with a line `TRACKS_PR_URL=<url>`; Station
follows each PR until it's merged or closed.

### Closing and coming back

Close Tracks from Quick Access, or with `tracks stop`. The tracks stay: the
next `tracks` lists the ones that were open and offers to reopen them, each
agent continuing its conversation where it stopped. Say no, and Resume in
Station brings any of them back later.

## Glossary

| Word | Meaning |
|---|---|
| track | one agent working on one task, in its own worktrees and window |
| Station | the tab listing your tracks |
| engine | an agent CLI a track runs on: Claude Code or Cursor |
| Quick Access | the popup on `Ctrl+b q` |
| Resume | start a track's agent again, continuing its conversation |
| Promote | give an Ask or Plan track its own worktrees, as a Work track |
| Archive | put a finished track away: its worktrees and local branches go, what was pushed stays, and the record moves to the archive |
| Derail | delete a track for good: worktrees, branches and record |

## Files

`tracks paths` prints them. By default:

- `~/.config/tracks`: `settings.yaml`, your themes, `tmux.conf` overrides
- `~/.local/state/tracks`: the tracks database, the daemon's socket and log,
  the worktrees
