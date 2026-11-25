# Trains

**Status:** agreed in discussion; the spike (step 0) is done, see Spike.
Being built, one PR per step under Order: steps 1 to 11 are built.

## Problem

- **A track's agent does everything itself.** Research, a failing CI job,
  pulling data from a ticket or logs, watching a PR: it all runs on the
  track's engine and model and fills its context.
- **The agents' own subagents are tied to them.** Claude's Task tool runs on
  Claude, inside the parent's sandbox, invisible to the user, and returns
  once. No other engine, no second opinion, no watching for an hour.
- **The reviewer already shows the way.** An agent of its own engine and
  model, in a hidden pane of the track, started by Tracks, reporting back
  through Tracks ([review.md](review.md)). It is a special case; it should
  be the first of a kind.

## Two kinds of child work

| | Independent track | Train |
|---|---|---|
| What | Unrelated work the track spun off ("take this idea into a new track") | A task the track hands off and gets back |
| How it starts | `tracks new <type>`, as now (#195) | `tracks train new <agent>` |
| Worktree | Its own | The parent's, read only |
| In the Station | Yes | No: on the parent's Platform |
| Lifetime | Its own | Ends with the parent |
| Control by the parent | None | Messages through the relay; it can end it |

The independent track stays as it is. It only gains a note on its details,
"Started from <track>", so the user sees where it came from. It needs a
migration. When that track is deleted (derailed), the note goes with it.

A third kind, the **side track**, is a track a main track orchestrates:
see Phase 2. The rest of this document, up to it, is about trains.

## Not in scope

- **Trains that write.** Only the track's own agent edits, commits and
  pushes. Two agents writing one worktree lose edits, see each other's
  half-done changes in builds and tests, and mix their commits. Work that
  writes in parallel goes to side tracks (Phase 2), each with a worktree
  of its own.
- **Nesting.** Trains open neither trains nor tracks.
- **The user posting to the relay.** The user reads it; they talk to a train
  in its overlay.
- **User-defined agents.** Later.
- **The Investigator and the Watcher in v1.** v1 builds trains with the
  Reviewer as the only one. The other two come in v2, as the first plugins
  ([protocol.md](protocol.md)).
- **Pinning a train beside the agent** (`join-pane`, as the reviewer pane
  does now). The overlay only, at first.
- **A cap.** None at first; a setting if trying it shows the need.
- **Token use and cost.** Trains add nothing to it. The engines' own apps
  and dashboards show what was spent.
- **Surviving a restart.** Trains end when Tracks or its daemon restarts;
  their messages stay in the relay.
- **A train's last message on the Platform.** The badge is enough; the
  Relay Station shows the messages.

## Trains (agents)

A train runs one of a fixed set of agents. The parent picks the agent and
writes the task; it can't make up an agent, or change its engine, model or
permissions. That keeps cost, permissions, prompts and the report's form
known.

**v1 has one: the Reviewer.** The Investigator and the Watcher below are the
candidates that come next; what each is for, its brief, its commands and how
the Watcher finishes are settled in their own design (Order, step 12).

| Agent | For | May | Reports |
|---|---|---|---|
| **Reviewer** | Reviewing the diff, answering questions on it | Read; no commands | `REVIEW OUTCOME:` through the hooks, as now |
| **Investigator** (later) | A failing test or CI job, research in the code, pulling data, reproducing a bug, a second opinion on a plan | Read, run commands; never edit | One report through the relay |
| **Watcher** (later) | CI, PR comments, a dev server's logs | Read, run commands; never edit; runs long | An event through the relay each time something happens; a report when its condition is met |

- **Settings → Trains (agents)** is today's Settings → Agents, renamed. A
  frame per agent, each with Agent and Model; v1 has the Reviewer's. The
  keys stay under `agents` in `settings.yaml` (`agents.reviewer`, later
  `agents.investigator` and `agents.watcher`), so existing settings keep
  working.
- **Each agent has a fixed brief**, golden-checked; the parent's task is
  added to it, marked as the task.
- `tracks review` stays the Reviewer's command. It is a train on the
  Platform and on the relay (see The Reviewer on the relay).

### Never edits: prevented where the engine can, detected everywhere

Tracks doesn't promise more than the engines can enforce.

- **Detected, on every engine.** The brief says never to edit, the edit
  tools are denied, and Tracks checks the worktree before and after, as it
  does for a review: tracked files, and untracked files that aren't ignored.
  Ignored build output is fine. A change is an `error` on the train and in
  its report.
- **Prevented, where the engine allows it:**
  - **The Reviewer** runs no commands: Claude in plan mode with the edit
    tools denied, Cursor in ask mode. Nothing to prevent beyond that.
  - **Claude running commands** (the spike): the train's own sandbox, with
    writes to the worktree denied (`sandbox.filesystem.denyWrite`, or an
    `Edit(<worktree>/**)` deny rule, through `--settings`), and
    `--permission-mode dontAsk`. Every command runs in the sandbox; a write
    to the worktree fails with "Operation not permitted"; a command outside
    the sandbox is refused without a question. An allowlist of commands
    alone isn't enough: managed settings that auto-allow sandboxed commands
    override it.
  - **Cursor running commands:** its CLI can't run commands while denying
    writes. Ask mode runs none, an allowlist asks the user for every other
    command, `--force` runs everything. Its permissions live only in a
    config file, the user's global one or one in the worktree. Detection
    covers it; whether a Cursor Investigator also asks for its commands is
    for that agent's design.

## Lifetime

- **Started by Tracks, not the parent.** `tracks review` (later also
  `tracks train new`) asks the daemon, which opens the train's pane through
  tmux, as for the reviewer now: the parent's sandbox doesn't apply, and no
  way around it opens.
- **Its own mode, always.** An agent started plainly takes the user's
  default permission mode (the spike's Claude started in auto mode), so a
  train always sets its mode. It still loads the user's global
  instructions, hooks, skills and MCP servers; the run ID keeps Tracks'
  hooks from mixing it up with the track, as for the reviewer now.
- **A tmux session of its own**, `tracks-train-<track>-<handle>`, one
  window and one pane. The overlay attaches a client to that session, so
  it needs one per train. It is never moved into the track's window: the
  reviewer's old route, `join-pane` into the right column and
  `break-pane` back, is gone (`join-pane` moves a pane out of its
  session). The window carries the track and the handle
  (`@tracks_train_track`, `@tracks_train`), for the header's buttons.
- **Its pane stays after its agent exits** (`remain-on-exit`), so the
  train is listed as `agent exited` until it's ended; a train that
  reported before it exited stays `done`.
- **It ends** with the parent, with **End train**, or with
  `tracks train end <name>`, which close its session. A finished train
  stays until then, so the parent and the user can still ask it.
- **A restart ends trains.** When Tracks or its daemon restarts, the
  track's trains are ended; their messages stay readable.

## The Platform

A narrow pane on the left of the track's window, a small Tracks program
built from the Tracks window's parts: `widget.BorderTitledFrame`, the theme's tokens,
the Station's status badges. It holds two frames: **Platform**, the trains,
and below it **Signals**: the Relay Station and the PRs' open comments.
(The sketch shows later agents beside the Reviewer.)

```
╭─ Platform ────────────────╮
│ Reviewer                  │
│  done   blocked           │
│ detective · Investigator  │
│  active                   │
│ watch-ci · Watcher        │
│  action required          │
│  [ Collect messages ]     │
╰───────────────────────────╯
╭─ Signals ─────────────────╮
│ Relay Station · 2 new     │
│ PR #12                    │
│  [ solve 3 comments ]     │
╰───────────────────────────╯
```

- **Always shown:** every track window has it, opened with the window and
  by the daemon's tick for a window that lacks one. With no trains the
  Platform frame says `empty`, muted; Signals always has the Relay Station,
  which opens empty on a relay without messages. Frame titles take their
  border's colour, entries the Station's list colour, and tmux draws no
  title over the pane.
- **No icons** at first. A train's name and agent, its status badge, and for
  the Reviewer its outcome as a second badge, as the Station shows a PR's.
- **Width:** "action required" and the frame need about 22–24 columns. A
  setting hides the sidebar; the track's info then says "2 trains · 1 needs
  you", and the PR buttons go back to the footer's top row.
- **The reviewer moves here, and its old UI went with step 7:**
  - the footer's Open/Hide reviewer and Close reviewer buttons;
  - moving its pane into the track's right column (`join-pane`) and back;
    it is seen only in the overlay;
  - showing its pane by itself for Claude's trust question (see Status);
  - `button.variant6`, which only Open/Hide reviewer uses. `button.variant5`
    stays, for End train.
- `badge()` moves from `ui/tracksview` to `ui/widget`, so the Station and
  the Platform draw the same badge.

### Signals: the Relay Station

The Signals frame has one entry, **Relay Station**, with the count of
messages the user hasn't seen. A click opens the track's relay in an
overlay, as a train's opens:

- A Tracks program in the popup, not an agent: the track's messages, oldest
  first, each with its time, sender, addressee and kind, the join and end
  notices among them. Long messages fold to their first lines and open on
  Enter.
- A message still waiting for its pointer is marked "not delivered".
- Read only: the user doesn't post (see Not in scope).
- **Esc**, `q` and `Ctrl+b Esc` close it. It is a plain program in a
  `display-popup -E`, not a nested client, so the popup gets both keys
  of `Ctrl+b Esc` and needs no tmux binding.
- Opening it marks the messages seen by the user, a reader of its own on
  the relay (`user`); it doesn't mark them read for any agent. The daemon
  counts what the user hasn't seen, for the entry's "N new".
- **Built (9a):** `internal/ui/relaystation`, run by the hidden `tracks
  relay-station`; `tracks relay-station open [--client <name>]` opens it
  over a client, 80% × 80%. The daemon's `relay-station` call is the
  relay, which messages wait for a pointer (from delivery's state in
  memory) and the user's mark; `relay-new` is the count. Adding a message,
  a reader's cursor moving and a pointer going or stalling tell the
  change feed, so it updates live.

### Signals: the PRs' comments

The footer's PR buttons ("PR #12 · solve 3 comments") move into Signals,
one entry per PR with open comments: the PR's label on one line, the
**solve** button under it, so a repo's name (multi-repo tracks) fits the
width.

- They work as now: a click sends the agent the prompt to answer the
  comments; once sent, the entry reads "sent to the agent", muted, until
  new comments come. The click is the user's, so it goes straight in, as
  Collect messages does.
- They take the default button colours; the variants (1 to 4, in turn) were
  the footer's.
- **The footer's top row is the track's info alone, centred again**, as
  before the reviewer's buttons took its sides.

### Status

The Station's badges, no new tokens:

| Train | Badge |
|---|---|
| Working | `active` |
| A question on its screen, or the trust prompt | `action required` |
| Report in | `done` |
| Exited without a report | `agent exited` |
| Failed to start, or the worktree check found an edit | `error` |

Two new values, on existing badge kinds, carry the Reviewer's outcome:
`pass` (info) and `blocked` (danger).

**A question a train asks the user**, Claude's trust prompt among them,
doesn't open its overlay by itself, as the reviewer's pane was shown
before step 7. The train shows `action required` and the user is
notified, once per run, under Settings → General → Notifications → "A
track needs you": "Tracks: @reviewer-527192 on <track> needs you". Unlike
other notices it goes even while the track is on screen, since the
overlay isn't, and the notifier's quiet time per track doesn't hold it
back. They open the overlay to answer.

### The overlay

A click on a train's name opens it over the track: `tracks train show
<name>` (a handle, or a prefix only one train has), which runs
`display-popup -E` with `env -u TMUX tmux -L <socket> attach-session` on
the train's session, 90% of the client's size, with a rounded border in
`overlay.border`. It is the train's own session, interactive: the user
answers its questions, asks it things, scrolls. Closing it only detaches;
the train runs on. `--track` and `--client` let the Platform, which runs
outside the agent's environment, name the track and the client.

```
╭───────────────────────────────────────────────────────────────╮
│ detective · Investigator   active         [ Hide ] [End train] │
│───────────────────────────────────────────────────────────────│
│   (the train's agent)                                         │
╰───────────────────────────────────────────────────────────────╯
```

- **A header row** is the inner client's status line, set on the train's
  session (`status on`, `status-position top`, `status-format[0]`): the
  train's handle and agent, its badges, and two buttons, ranges handled by
  the footer's `MouseDown1Status` binding, which now passes the clicking
  client too.
  - **The badges** (status, and the Reviewer's outcome) are a session
    option, `@tracks_train_badge`, that the daemon's tick rewrites when they
    change, redrawing the attached clients' status lines. Their colours are
    global options set from the `state.*` tokens, as the buttons' are.
  - **Hide** (`button.variant1`) detaches the client: the popup closes, the
    train runs on.
  - **End train** ends it, in `button.variant5`, as Close reviewer did,
    once the user confirms in a popup over the overlay.
- **`Ctrl+b Esc`** hides it too: a prefix binding that runs `tracks
  trackwin hide <client> <session>`, which detaches the client only when
  its session is a train's. A bare Esc stays the agent's (it interrupts);
  `Ctrl+b x` would kill the pane. It does nothing outside an overlay.
- **No asking a train while it's attached.** A reply to the user would pass
  for the answer; tmux says whether a session has a client. `tracks review
  --ask` and a `question` through the relay both refuse then, and refuse a
  train whose agent exited.

## The relay

How a track and its trains talk: messages kept in Tracks' database, routed
by Tracks. A relay station more than a chat.

- **Handles.** Each track and train has one, given by Tracks.
  - The track is **`track`**: one per relay, and every train addresses it.
  - A train is its agent's prefix and six random digits, new each time it
    starts: `reviewer-527192`. A new review is a new handle, so the Relay
    Station keeps an old reviewer's messages apart. The prefix is set per
    agent in Settings → Trains (agents), its agent's name by default.
  - `--to` and `tracks train end` take a handle, or a prefix only one of
    the track's trains has (`reviewer`); with two, Tracks names both.
  - Notices come from **`tracks`**, Tracks itself.
- **Tracks stamps the sender.** In v1 the only sender running commands is
  the track's agent, named by `$TRACKS_ID`; Tracks posts for the reviewer,
  and a process with the review marker is refused. Once trains run
  commands (step 12), each agent's environment carries a token, as the
  reviewer's run ID; the daemon maps it to the handle. Nobody sends as
  another.
- **Star layout.** A train sends only to its track; the track sends to its
  trains. Trains don't talk to each other in v1; addressing by handle
  leaves room for it later (see Later).
- **Who's on the platform.** `tracks relay who` lists the track's handles:
  each one's agent, the first line of its task and its status. Each train's
  brief says other trains may be there and how to list them.
- **Join notices.** When a train starts, Tracks adds a notice for the
  track's other trains and the track: "@watch-ci joined: Watcher, <first
  line of its task>". It sends no pointer: a notice wakes no one, and comes
  with the reader's next `relay read`. A notice that a train ended comes the
  same way.
- **Addressed with a flag**, never found in the text:
  `tracks relay send --to big-boss --kind report` with the message on stdin.
  Reports quote code (`@Override`), PR comments (`@someone`) and handles
  in passing.
- **Kinds:** `report`, `event`, `question`, `answer`. Tracks reads them for
  the badges: an unanswered `question` is `action required`, a `report` is
  `done`.
- **Reading:** `tracks relay read` prints the reader's unread messages, each
  fenced with its sender and kind, as the train's words, never
  instructions: the rule the reviewer's GitHub text follows. Long ones show
  their first lines; `tracks relay show <id>` prints one whole.
- **The user reads it** in the Relay Station; they don't post.
- **A train that ends its turn without a report** gets a fixed pointer, as
  any delivery: "Send your report with `tracks relay send --to <track>
  --kind report`." A train that exits without one leaves the track a
  notice: "@detective exited without a report". (Not the Reviewer: its
  report comes through the hooks.)

### The Reviewer on the relay

The Reviewer runs no commands, so it can't `relay send` or `relay read`.
Tracks speaks for it:

- **Its handle is new with each review** (`reviewer-527192`), kept in its
  review folder. Tracks posts "@reviewer-527192 joined" as it starts, and
  "left" when it's ended or a new review replaces it.
- **Its report and answers go on the relay** as its messages, kind
  `report` or `answer`, taken from the hooks as now (Claude's Stop hook;
  Cursor's print-mode turn). The track gets a pointer (step 5), as for any
  message.
- **A question for it** is `tracks review --ask`, or `tracks relay send --to
  reviewer --kind question`; both post it as the track's question. Tracks
  types it into the reviewer's session, behind the fixed lead-in `--ask`
  uses: it comes from the track, never from another train (the star
  layout). Other kinds to it are refused. The delivery blockers apply
  (step 5).
- **`tracks review --wait` and `--ask`** wait on the review's files and
  print as before, so the prompts that use them don't change. What they
  print is marked read on the relay, so `relay read` doesn't repeat it;
  marking is a reader's cursor, so what came before it counts as read too.
- The Relay Station shows the reviews and the questions and answers.

### Delivery

An idle agent does nothing until it gets input, so Tracks tells the
addressee that something came. It types a fixed pointer into its pane,
never the message:

> 2 messages from @detective: run `tracks relay read`

Typed text reads to the agent as the user's own words; the message comes
back as a command's output instead, marked as the sender's.

Tracks holds the pointer while anything blocks it:

| Blocker | Clears when |
|---|---|
| The agent is in a turn | Its Stop hook |
| A question on its screen (`action required`) | It's answered |
| The user is typing in its pane or overlay | No key for a few seconds (`client_activity`) |
| Text in its input | The input line is empty, read from the screen as the markers are |
| The user scrolls back in its pane (copy mode) | They leave the scrollback (`#{pane_in_mode}`) |

A turn starts with a prompt (Claude's `UserPromptSubmit`, Cursor's
`beforeSubmitPrompt`) and ends as the agent stops (`Stop`, `StopFailure`,
`stop`); a subagent's hooks are its turn going on. Tracks keeps it in
memory: after a restart it doesn't know, and counts the agent as in a
turn until its next one ends, or Collect messages sends the pointer.

From the spike:

- **Typing:** `client_activity` changes on keystrokes only, not on output
  or a resize, in whole seconds. It is per client: the check covers every
  client on the target's session, an overlay's among them.
- **An empty input:** on both engines the placeholder is drawn dim
  (`ESC[2m`) after the prompt marker (`❯` for Claude, `→` for Cursor), and a
  draft in normal text. Read with `capture-pane -e`: the input is empty
  when everything after the marker is dim or blank, whatever the
  placeholder says. Claude's input can span several lines: all lines
  between its two rules count.

- **Retried on the footer's tick**; messages waiting for one agent go as one
  pointer.
- **Collect messages.** While a pointer is held, the sending train's entry
  on the Platform shows **[ Collect messages ]**. A click sends it now,
  past the turn, the typing, a draft or a retry's wait: the user vouches
  for the input. A question on the agent's screen still holds it, since
  the pointer would answer it; a pane scrolled back leaves its scrollback
  first, so the Enter submits. It stops the retries.
- **Only what waited:** a message is pointed to once it's 3 s old and
  still unread, so a `tracks review` that prints it marks it read first;
  notices never are. A pointer goes once per message.
- **A paste, then Enter:** the pointer goes in as one bracketed paste
  (`set-buffer` and `paste-buffer -p` in one tmux call), so keys typed
  meanwhile can't land inside it; Enter follows after the short delay the
  agent CLIs need to take it as a submit, not a new line.
- **Stall check:** an agent whose turn doesn't start within 5 s didn't
  take the pointer: its messages count as not pointed to, and the pointer
  goes again after a minute. A new agent in the window (a resume, a
  restart, a promote) is pointed to the unread messages again.
- **Trains get pointers too:** a question from the track to a train is
  delivered the same way, with the same blockers.
- **The user is notified** when a train reports, answers, asks or sees
  something, through the notification channels, under Settings → General
  → Notifications → "A train reports or answers": the train's handle, the
  track, and the message's first line. Not for the track they're looking
  at, as every notice. A train needing them (its trust question) tells
  them under "A track needs you" (see Status).
- **Waiting is still there.** `tracks relay wait` blocks until a message
  for the caller comes. A Claude parent can run it in the background; a
  train can end its task on it, to be there for questions.

The safeguards follow what Herdr does for agent prompts (bracketed paste
and Enter as one, refusing a blocked agent, the stall check). Herdr pushes
nothing into the orchestrating agent; it waits and reads.

## Control surface

| Command | Does |
|---|---|
| `tracks train new <agent> --task <text>` | Later, with the Investigator: starts a train; `--name` sets its handle, `--task-file -` reads the task |
| `tracks train list` | The track's trains, their agent, handle and status |
| `tracks train show <name>` | Opens one in its overlay, over the client; returns once it closes |
| `tracks train end <name>` | Ends one and closes its session |
| `tracks relay send --to <handle> --kind <kind>` | Sends stdin as a message |
| `tracks relay read` | Prints the caller's unread messages, marks them read |
| `tracks relay show <id>` | Prints one message whole |
| `tracks relay wait` | Blocks until a message for the caller comes |
| `tracks relay who` | The track's handles: agent, task's first line, status |
| `tracks review …` | As now |

The track's prompt gains a Trains paragraph: when to start one, which
agent, the relay commands. Prompts are golden-checked; its wording is agreed
first.

## Spike

Step 0, run on a throwaway tmux server and the real agent CLIs:

| Check | Result |
|---|---|
| A popup with a nested client on the train's session | Works: typing reaches the train, not the track |
| Hide in the header (a status-line range) | Detaches the popup's client; the train runs on |
| `Ctrl+b Esc` | Reaches the popup's client and hides it |
| Claude and Cursor at popup size | Claude redraws cleanly; Cursor draws inline and leaves old lines above, harmless in the overlay |
| `client_activity` | Keystrokes only (see Delivery) |
| An empty input | The dim placeholder (see Delivery) |
| Claude: commands without edits | Its sandbox with the worktree denied, and `dontAsk` (see Never edits) |
| Cursor: commands without edits | Not possible with its CLI (see Never edits) |

## Order

Each step a PR of its own.

1. `badge()` to `ui/widget`.
2. Settings → Trains (agents): the rename.
3. Trains: `internal/train` (handles, agents, status), the Reviewer as the
   first, read from tmux and its run as now; `tracks train list|end`.
   Ending with the parent, as hidden panes end now. No migration: trains
   end on a restart, so tmux stays their record.
4. The relay, in three PRs:
   - 4a: the first migration, `internal/relay`, handles (prefix, digits,
     resolving a prefix), `tracks relay send|read|show|wait|who`;
   - 4b: the Reviewer on it: its handle, its reports and answers posted as
     its messages, `--ask` as a question, join and end notices; `--wait`
     and `--ask` print as now and mark what they print read;
   - 4c: the prefix in Settings → Trains (agents).
5. Delivery: the pointer, the blockers, the tick, the stall check,
   notifications.
6. The Platform: the pane, the list, the badges; the Reviewer in it.
   Built: `tracks platform` in a pane `PlatformWidth` (26) wide on the
   window's left, opened and closed by the daemon's tick
   (`SyncTrains`), which also tells the change feed when a track's
   trains changed, since their status comes from tmux and their runs,
   not the store. Clicking a train comes with step 7.
7. The overlay: a session per train in place of a window in
   `tracks-back` (`join-pane` moves a pane out of its session, so the two
   can't come apart), the popup, the header with Hide and End train,
   `Ctrl+b Esc`. A train's pane stays after its agent exits, so the
   Platform can show `agent exited`; until then a train is listed while
   its pane is open. Today's reviewer UI goes: the footer's reviewer buttons,
   `join-pane`, the pane shown for the trust question, `button.variant6`.
   Built: `tracks train show`, which a click on a train in the Platform
   runs; the trust question is told under "A track needs you"; a theme file that still sets `button.variant6` loads, the
   token skipped.
8. Collect messages. Built: delivery notes whose messages a held pointer
   waits for and tells the change feed; `relay-collect` sends it.
9. Signals, in two PRs:
   - 9a: the Relay Station: the program, "not delivered", the user's
     mark and the count of new messages, the popup. Done.
   - 9b: Signals in the Platform, its entry opening the Relay Station; the
     PR buttons moved in from the footer, the track's info centred again.
     Done: the sidebar shows for a train or a PR's comments, Signals under
     the Platform, the track's info centred when no button is left. The
     setting that hides the sidebar, and the PR buttons back in the footer
     then, comes later.
10. The track's prompt: the relay paragraph (reading messages when a pointer
    comes). Done, for Work and Review tracks; Plan tracks may get it later.
11. The "started from" note on independent tracks. Independent of the rest.
    Done: `parent_id` on `tracks`; a parent that's deleted leaves no note.
12. The Investigator and the Watcher: **moved to v2**, built as the first
    plugins on the open interface of [protocol.md](protocol.md), maybe in
    a repo of plugins. What's settled here still holds for them: their
    commands (prevented where the engine can, detected everywhere), the
    worktree check in the caller's sandbox (the daemon runs no git), the
    Watcher's finish condition in its task. v1 ends with the Reviewer as
    the only train.

Later: a cap setting, user-defined agents, pinning beside. Writing in
parallel is Phase 2, side tracks.

Trains talking to each other, if a case needs it, with these rules:

- Only `question` and `answer` between trains; reports and tasks still go
  through the track.
- The track sees every message between trains in its `relay read`, without
  a pointer for them.
- A limit per pair of trains, say 5 messages; after it Tracks refuses, and
  they go through the track.
- Messages stay data, fenced with their sender.

## Phase 2: Side tracks

**Status:** agreed in discussion, not built; after the train steps above.

Tracks running side by side, each on its own worktree and branch with its
own review and PR, while one track plans, checks for overlaps and merges,
works well when the user drives it by hand. Phase 2 lets a track drive it:
a **main track** orchestrates **side tracks** that write code in parallel,
and they talk through its relay so the main track keeps track of the work,
the PRs and the conflicts.

| | Train | Side track |
|---|---|---|
| What | An agent that reads and reports: the reviewer, an investigator | A track that builds: code, commits, a PR |
| Runs in | A hidden pane of its track, shown in the overlay | A track window of its own |
| Worktree | Its track's, read only | Its own, on a branch of its own |
| Review and PR | None | Its own `tracks review`, its own PR |
| In the Station | No | Yes, under its main track |
| On the main track's Platform | Yes, as a train | Yes, under "Side tracks"; a click switches to its window |
| On the relay | Yes | Yes, with a handle, `side-000123` by default |

A **main track** is any track with a side track; there's no kind of its
own. "Side track" is always two words: "sidetracked" means something else.

### A side track

- **Started by its main track**: `tracks side new --task <text>` (or
  `--task-file -`), a Work track with the main track's repos by default
  and `--repo` to pick others; the task is its prompt. `tracks side list`
  lists them with their status and their PR.
- **A track like any other**: its own worktree, branch, window, agent,
  pre-push review, PR, comments and CI status, as Tracks has for every
  track. Its parent is the main track, the link step 11 added ("Started
  from"), and it joins the main track's relay.
- **It reports through the relay**: "PR #231 is ready, review passed",
  "blocked on …", questions. The main track gets the pointer, checks the
  PR against the task it gave, and tells the user.
- **Lifetime**: its own. It doesn't end when the main track pauses, and
  its PR stays open as any track's does.
- **The main track waits for its side tracks**: it can't be ended while
  a side track is unsettled, so none is left headless, linked to a main
  track that's gone. Settled is ended, its PR merged or closed, or none
  opened. Ending it says which side tracks are left, for the user to
  settle (or end) first.
- **No nesting**: a side track starts no side tracks of its own.

### Pull requests

- **One PR per side track**, opened as the track rules say (after its
  review passes), **as a draft**. The main track checks it against the
  task it gave and marks it ready, unless the repo's own rule (the
  Repositories tab's Draft PRs) says PRs stay drafts: then the main track
  never promotes it, and the user does.
- **Restacking**: when the PR below a stacked one merges or changes, the
  side track's own agent rebases, woken by `@github`'s event ("#232
  merged: rebase onto main"); GitHub on the relay comes first for this.
- **Stacked when the work builds up**: a side track can be based on the
  main track's branch or on an earlier side track's, its PR on that
  branch, so GitHub's stacked pull requests carry the order and retarget
  each PR as the one below it merges. `tracks side new --on <side track>`
  stacks; without it, a side track branches from the repo's base.
- The main track sees its side tracks' PRs on its Platform, with their
  state, as a track's own PRs show in the Station.

### Conflicts

- **Side tracks talk to each other** to settle them, under the rules
  parked for trains above: only questions and answers between them, the
  main track sees every message, at most 5 messages a pair, messages stay
  data fenced with their sender. Tasks and reports go through the main
  track.

### The Station

Side tracks are grouped under their main track, indented, with a corner
line before each:

```
api-cache            Work    active     PR open
└ side: relay-store  Work    active
└ side: relay-cli    Work    done       PR open
docs-fix             Work    done
```

Filters and sorting keep a side track with its main track; a side track
whose main track is filtered out shows on its own with "side track of
<name>" in its details.

### GitHub on the relay

Tracks learns about its PRs by polling GitHub: a laptop's daemon can't
take webhooks without a tunnel, repo admin rights and a fallback for
while it sleeps. The polling gets cheaper and its news reaches the agents:

- **Polling** (built): one GraphQL query a minute reads every branch's
  PRs and every PR not merged or closed (state, mergeability, CI checks,
  open review threads), in aliases of 25 per call, where there were a
  `gh pr list` per branch, a `gh pr view` per PR and a threads query per
  PR. Signals' PR buttons count from the same answer. A repo's GitHub
  name is asked of gh once per checkout. GitHub charges a query by the
  most it could return, so a branch's PRs come without their review
  threads (ten times the cost); a PR found there is read with its
  threads, by its link, from the next poll on. A poll of a branch and a
  PR costs 1 point of the 5,000 an hour.
- **Not the notifications endpoint**: it was the plan, but it misses what
  matters most here. A PR that starts to conflict as its base moves
  notifies nobody, CI notifies only for runs the user triggered and only
  if they turned that on, and the rest only for threads the user
  subscribes to. It could still wake a poll early (its `304`s are free of
  the rate limit) if a minute proves slow.
- **`@github` on the relay** (built): a sender of its own, reserved as
  `track` and `tracks` are, posting `event` messages to the open Work
  track (or side track) whose PR it is, once per change: "PR acme/api#231
  was merged into main", "… was closed without merging", "PR
  acme/api#234 has conflicts with main", "CI failed on PR acme/api#234
  at 0123abc: lint, test" (again after a push that fails again), "3
  review threads on PR acme/api#234 wait for an answer, 2 of them new".
  Unlike notices, they wake the agent: delivery points it to them, so it
  can rebase, fix the check or answer the comments on its own. The Relay
  Station shows them. What the poll last read of each PR (mergeability,
  checks and their commit, open threads) is kept in the store, so a
  restart repeats none; the first count of a PR's threads posts nothing,
  as they may have waited long.
  Held back, as a train's messages are, they offer Collect messages under
  Signals' Relay Station.
- **Facts only**: `@github` posts what Tracks reads (states, counts, check
  names), never a comment's text, which the agent reads on GitHub through
  the comments flow, under the same fenced-data rule.
- **With side tracks**: a side track's PR merging tells its main track it
  can restack the next; a conflict goes to the side tracks it concerns.

### Order

1. GitHub on the relay: `@github`'s events, and one GraphQL query a
   poll in place of the per-PR calls (built). First: it's useful for a
   track's own PRs, and restacking needs it.
2. The model: a side track is a track with a main track (the step 11
   link) on its main track's relay; `tracks side new|list`; no nesting;
   the main track can't end while a side track is unsettled.
   Built, in two PRs:
   - 2a, the model: `tracks.side`, the side track's handle on its main
     track's relay (`side-000123`), marks it; `parent_id` alone stays
     "Started from". A column, not a flag: the handle outlives restarts,
     unlike a train's, and the relay's messages name it.
     `internal/side` decides what a side track is and when it's settled:
     ended, and its PRs merged or closed, or none opened. End, Archive and
     Derail refuse a main track with an unsettled side track, naming
     each and why; auto-archive skips it. A window closed some other way
     can't be refused: the main track ends, its side tracks run on.
     `tracks side new` is `tracks new work` with the side flag: the
     daemon refuses it from a side track and for a main track that
     ended, the command under the review marker, as `tracks relay`.
   - 2b, the relay: a side track reads two relays, its own as `track`
     (its reviewer's) and its main track's as its handle; `relay read`,
     `wait`, `show` and delivery merge the two (message numbers grow
     across relays, so one cursor orders them). `--to track` in a side
     track sends to its main track, from its handle: identity is still
     `$TRACKS_ID`, so a side track can't send as its main track. The
     main track sends to a side track by handle or prefix, as to a train;
     an ended one is refused. `relay who` lists the side tracks; a side
     track sees its main track as @track, itself, and its own trains.
     Tracks posts "@side-000123 joined: side track <name>, <task's first
     line>" as one starts or resumes, and "left: it ended." (or "Tracks
     stopped.") as it closes. `relay wait` now sleeps through notices,
     as delivery does: a join notice wakes no one.
     The main track's Relay Station marks only its own pointers "not
     delivered", not those of messages to its side tracks.
3. Draft PRs the main track promotes, as the repo's rule allows.
4. The Platform's "Side tracks" list; a click switches to the window.
5. The Station's grouping.
6. Stacking: `--on`, the PR's base; restacking on `@github`'s events.
7. Side tracks talking to each other.
8. The main track's prompt: a "Side tracks" paragraph, its wording agreed
   with the user first, as every prompt.

### Later (v3)

- **Overlap notices**: Tracks spotting side tracks that change the same
  files, with no model involved, and telling them and the main track
  ("@side-000123 and @side-000456 both change internal/relay/relay.go").
  It needs its own planning: where the changed files come from (the
  daemon runs no git, so each side track's hooks might report them from
  its own sandbox), how often, generated files, and what a notice asks
  of the agents.
- **A cap** on side tracks running at once, a setting: each is a full
  agent with its own reviewer, and the cost adds up.

## Open

- Claude on a machine without managed settings: the train switches its
  sandbox on and auto-allows it itself, through `--settings`; to confirm
  with the first plugins that run commands (v2).
- The hosts a command-running train's sandbox lets it reach (`gh`, `curl`).
- The Investigator's and the Watcher's design: v2, in
  [protocol.md](protocol.md).
