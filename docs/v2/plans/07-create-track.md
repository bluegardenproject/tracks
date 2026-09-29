# Plan: creating tracks

**Status: built.** Part of the [v2 masterplan](../masterplan.md), chunk 5 (real tracks). The New track form ([06-add-track.md](06-add-track.md)) creates real tracks: the v2 daemon makes the worktrees and starts the engine with v1's prompts in the track's window.

## What users get

- **Create creates the track.** The form checks the fields as today, then shows the daemon's progress in its hint row ("Fetching origin/main in tracks…", "Creating the worktree…", "Starting Claude Code…").
- **When it's ready,** the popup closes and the user lands in the new track's window, on the agent pane.
- **Esc while it's being created** closes the form, and creation goes on. The status line says when it's ready ("rate-bug is ready: Ctrl+b 3") or why it failed.
- **A failure** rolls back what was made so far: the window, the worktrees and a Work track's new branch. The form stays open with the error, and Create tries again. Nothing is saved.
- **The engine:** new tracks run on Claude Code when it's added, else on Cursor. The form shows what runs the track above its buttons ("Runs on Claude Code, model opus"). With no engine added, Create says "Add an engine on the Engines tab first." A default engine per track type comes later.
- **The track's window:**
  - The agent pane runs the engine with v1's prompt.
  - A terminal pane opens beside it when Terminal is ticked.
  - The window is named after the track's name, or else, for Doc, from the document's name, or else from the prompt, as in v1. A name already taken gets `-2`, `-3` and so on.
- **Station** lists the tracks from the database while their window is open: name, kind, repos with branch and worktree, engine, model and session. End closes the window as before; the daemon records it.
- **Station without tracks** keeps its layout: the Tracks frame with an empty list and an **Add new Track** button in its centre, which opens the New track form (Enter does too), and the Fast Track frame on the right.
- **Repos in the form** are a select list instead of a checkbox list:
  - The field starts as **Select repos**. Enter or a click opens the picker, where typing filters, and Space or a click ticks any number of repos. **OK** or Enter closes it, and so do Esc and a click outside; the ticks stay either way.
  - The ticked repos show in a row under the field, each with an ✕ that removes it. Left and right walk them, and Enter or Backspace removes one.
  - Review's **Repo** is the same field showing its one repo; the picker chooses one.
- **Settings** gets a **Track** section above Fast Tracks, a placeholder until each track type gets its defaults.
- **Overlay tokens:** the picker, and the popups' background and frames, use their own `overlay.*` tokens, which start from the values drawn before. `bg.overlay` becomes `overlay.bg`. What the popups hold keeps its tokens.

## How a track is created

The same steps as v1's `handleNew`:

1. **Check** the request again in the daemon: the repos exist, the kind's rules hold, the document can be read.
2. **ID:** `YYYYMMDD-HHMMSS-<6 hex>`.
3. **Session:**
   - Claude gets a UUID made locally, passed with `--session-id`.
   - Cursor gets a chat from `agent create-chat`, a network call with a 30 s limit.
4. **Worktrees,** for Work and Review, one per repo in `<data>/worktrees/<id>/<repo>`, after fetching `origin/<base>` with retries:
   - **Work:** a new branch `tracks/<last 6 of the ID>` from `origin/<base>`; `-2` to `-50` when taken. The agent renames it, as in v1.
   - **Review:** the PR's `pull/<n>/head` or the branch is fetched, then checked out detached at `FETCH_HEAD`, shown as `pr/<n>` or the branch name.
   - **Ask, Plan, Doc:** no worktree. The agent reads the primary checkouts, told to leave them alone.
5. **Command line,** per engine as in v1:

| | Claude Code | Cursor |
|---|---|---|
| Work, Review | `--permission-mode auto` (`default` with auto mode off) | `--force` (none with auto mode off) |
| Ask, Plan | `--permission-mode plan` | `--mode ask` or `--mode plan` |
| Doc | `--permission-mode default` | no `--force` |
| Repos | `--add-dir` per repo, plus the document's folder | the first repo is `--workspace`, the rest `--add-dir` |
| Model | `--model` when the engine has a default model | the same |
| Session | `--session-id <uuid>` | `--resume <chat id>` |

   The pane starts where v1 starts it: Claude in the document's folder for Doc, else the first repo; Cursor in the first repo, else the document's folder; both else in the home folder. It runs v1's wrapper: `TRACKS_ID` and `TRACKS_SOCKET_DIR` are set, and a login shell takes over when the agent exits.
6. **Window:** `trackwin.Open` with the command, the name and the terminal pane.
7. **Save** the track and its repos in one transaction.
8. **Switch** the client that pressed Create to the window.

## Prompts

- **v1's text, word for word** ([Decisions](../masterplan.md#decisions)):
  - Claude's `taskSuffix`, `docReviewTemplate`, `docReviewBrief`, `reviewCandorSuffix`
  - Cursor's `taskSuffix`, `docReviewSuffix`, `docReviewBrief`, `reviewCandorSuffix`
  - the shared parts in `internal/agent`: `ReadOnlySuffix`, `DraftPRSuffix` (from the repo's Draft PRs setting), `DevServerContract`, `TerminalContract` and the `Doc*` texts
- **Names that change,** so v1's and v2's helpers don't overwrite each other: `tracks-reviewer` becomes `tracks-v2-reviewer`, and `tracks-docs-reviewer` becomes `tracks-v2-docs-reviewer`. The masterplan lists them.
- **Commands the prompts mention that v2 doesn't have yet:** `tracks up`, `services`, `down`, `url`, `terminal`, `review` and `promote`.
  - In a v2 track, `tracks` on `PATH` is a small script in `<data>/bin` that runs `tracks --new-app`. These commands fail with "unknown command", and the prompts already tell the agent to report such a failure.
  - v1's commands look for their daemon at `<TRACKS_SOCKET_DIR>/sock`; v2's socket has another name, so they can't reach it either.
- **The reviewer subagents** are v1's files with the name changed: `~/.claude/agents/tracks-v2-reviewer.md` and `tracks-v2-docs-reviewer.md`. The daemon writes them when it starts, and never over a file without Tracks' ownership marker, as v1 does.
- **Not yet:** the add-repo skill and the Cursor rule. They describe commands v2 doesn't have. v1's Cursor rule, when v1 is installed, loads in v2 tracks too, since it only checks `TRACKS_ID`.

## The daemon

- **Start:** `tracks --new-app daemon` (hidden), started with `run-shell -b` on the Tracks tmux server, so it has the server's environment, the same `PATH` the Engines tab checks.
  - `./tracks --new-app` starts it.
  - So does any client that finds it gone, or older than its own binary (a different version, or the same path with a changed file), as v1 does. An older daemon is asked to shut down first.
- **One per user:** a lock on `<data>/daemon.lock`. The socket is `<data>/daemon.sock`.
- **Stop:** `./tracks --new-app stop` asks it to shut down before stopping the tmux server. It also exits when the server is gone, checked every 2 s.
- **Log:** `<data>/daemon.log`.
- **Protocol:** one JSON request per connection. The reply is progress lines, then one result or error line. Methods for now: `ping`, `shutdown`, `create`, `list`, `end`.
- **Creation runs in the daemon's own context,** not the connection's, so closing the form doesn't stop it. When nobody is listening any more, it reports to the client that asked, with `display-message -c`.
- **Watching:** every 2 s, a track whose window is gone gets its `closed_at`. That is all the supervision for now.

## Database

`store/migrations/0002_tracks.sql`:

```sql
CREATE TABLE tracks (
  id          TEXT    PRIMARY KEY,               -- 20260928-151500-a1b2c3
  kind        TEXT    NOT NULL CHECK (kind IN ('work', 'ask', 'plan', 'review', 'doc')),
  name        TEXT    NOT NULL,                  -- the window's name
  engine      TEXT    NOT NULL,                  -- claude, cursor
  model       TEXT    NOT NULL,                  -- '' is the engine's own default
  session_id  TEXT    NOT NULL,
  prompt      TEXT    NOT NULL,                  -- as the user wrote it, without Tracks' additions
  review_ref  TEXT    NOT NULL DEFAULT '',       -- Review: the PR link or branch
  document    TEXT    NOT NULL DEFAULT '',       -- Doc: the resolved path
  candor      INTEGER NOT NULL DEFAULT 0,        -- Review, Doc: 1 to 10
  opinion     INTEGER NOT NULL DEFAULT 1 CHECK (opinion IN (0, 1)),      -- Doc's sections
  claim_check INTEGER NOT NULL DEFAULT 1 CHECK (claim_check IN (0, 1)),
  terminal    INTEGER NOT NULL DEFAULT 0 CHECK (terminal IN (0, 1)),
  created_at  INTEGER NOT NULL,                  -- Unix milliseconds, UTC
  closed_at   INTEGER                            -- the window closed
) STRICT;
CREATE INDEX tracks_open ON tracks (closed_at, created_at);

-- name, path and base are copies, so a track's history survives
-- renaming or removing its repo.
CREATE TABLE track_repos (
  track_id  TEXT    NOT NULL REFERENCES tracks (id) ON DELETE CASCADE,
  position  INTEGER NOT NULL,                    -- the order picked; 0 is where the agent starts
  repo_id   INTEGER REFERENCES repos (id) ON DELETE SET NULL,
  name      TEXT    NOT NULL,
  path      TEXT    NOT NULL,                    -- the primary checkout
  worktree  TEXT    NOT NULL DEFAULT '',         -- '' for Ask, Plan, Doc
  branch    TEXT    NOT NULL DEFAULT '',         -- tracks/a1b2c3, pr/123 or the reviewed branch
  base      TEXT    NOT NULL,
  PRIMARY KEY (track_id, position)
) STRICT;
CREATE INDEX track_repos_repo ON track_repos (repo_id);
```

- **No status column:** statuses wait for the [status model](../masterplan.md#track-status-to-be-designed). The table holds facts only; a track is open while `closed_at` is empty.
- **Reliability:**
  - A track is saved once, after its worktrees and window exist, in one transaction with its repos, so there are no half-made rows.
  - The daemon writes tracks and the Tracks window still writes repos. Two writers are safe with WAL, `busy_timeout` and immediate transactions at this write rate; repos move behind the daemon later.
  - A daemon killed mid-creation can leave a worktree folder without a row. Clean (a follow-up) removes those.
- **Performance:** Station's list is an indexed query on open tracks plus one window listing, every 2 s.
- **Permissions:** prompts and session IDs are stored from now on, so the database follow-up is done here. The data folder becomes 0700, and the database 0600; SQLite gives its WAL and shared-memory files the database's mode. Backups get 0600 too.

## Packages

New, to agree first:
- `track/`: the domain, pure: `Track`, `Kind` (and which kinds have worktrees), `Repo`, and the window-name rule.
- `tracks/`: the use cases, Create and End, over small interfaces (worktrees, windows, store, sessions).
- `workspace/`: a track's git worktrees: fetch, add, review checkout, branch names, removal on failure. Uses v1's `internal/git` unchanged, a leaf package.
- `agents/claude/` and `agents/cursor/`: each engine's command line, prompt texts and session ID; Claude's also writes its reviewer subagents.
- `daemon/`: process lifecycle, the socket server, and routing requests to `tracks`.
- `rpc/`: request and response types and the client, shared by the CLI, the popups and the daemon.

Changed:
- `agents/`: the shared parts, meaning command-line quoting, the pane wrapper and the shared prompt texts.
- `store`: the tracks tables and queries, and the permissions.
- `settings`: the engine new tracks run on.
- `theme`: the `overlay` tokens; the picker, `ui/quickaccess` and `ui/themecreator`'s previews use them.
- `trackwin`: a `@tracks_id` window option, and v1's kind names.
- `platform`: the socket, lock, log and `bin` paths.
- `ui/addtrack`: Create, progress, the engine line, the repos' select list, and the overlay frames.
- `ui/widget`: the picker ticks several items when asked, with an OK button.
- `ui/tracksview`: Station's empty state, and Settings' Track section.
- `ui/source`: tracks from the daemon, and the Repositories tab's in-use check by repo ID.
- `cli`: the daemon command, start and stop, the popup, and Station's source.

## Not in this plan

- Resume, promote, Clean (removing a track's worktrees), and a better word than End.
- Statuses, attention, PR detection and cost: with the status model and hooks (chunk 6).
- Provisioning (env files, dependencies), submodules, dev servers and ports.
- Failed creations saved as drafts.
- The add-repo skill, the Cursor rule, and the commands the prompts mention.
- Default engines and models per track type; choosing the engine or model in the form.
- Repos written through the daemon; renaming a repo that tracks use.

## Tests

- **Prompt parity:** `agents/claude` and `agents/cursor` build the same prompt and command line as v1's `BuildOptions` and `ShellCommand`, with the listed names swapped. This covers every kind: Ask with and without repos, Review with candor, Doc with sections off, and draft PRs for some repos and for all. The reviewer files match v1's templates.
- **`workspace`,** on a temp repo with a bare origin:
  - a branch from `origin/<base>`, and the `-2` suffix
  - reviewing a branch and a `pull/<n>/head` ref
  - removal after a failure
- **`track`:** window names from the name, the prompt and the document.
- **`store`:** the migration, a round trip, the open list, and closing.
- **`tracks`:** Create with fakes: each kind's steps, rollback when each step fails, and a name already taken.
- **`daemon` and `rpc`:** ping, progress then result, shutdown, and a second daemon refused by the lock, on a short temp socket path.
- **`ui/addtrack`:** Create sends the request and shows progress; a failure keeps the form; Esc while creating closes it.
- **End to end, isolated:** create a Work track against a temp repo, with a fake `claude` on `PATH` that prints its arguments.

## Delivery

One branch, `feat/v2-create-track`, with these commits: this plan; `track` and `store`; the prompts in `agents`; `workspace`; `tracks`; `rpc` and `daemon`; the CLI and the form; Station; docs.
