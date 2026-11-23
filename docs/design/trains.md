# Trains

**Status:** agreed in discussion; the spike (step 0) is done, see Spike.
Not built. One PR per step under Order.

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
"started from <track>", so the user sees where it came from. It needs a
migration.

The rest of this document is about trains.

## Not in scope

- **Trains that write.** Only the track's own agent edits, commits and
  pushes. Two agents writing one worktree lose edits, see each other's
  half-done changes in builds and tests, and mix their commits. A writing
  train would need a worktree and a merge of its own; it waits until a real
  need shows.
- **Nesting.** Trains open neither trains nor tracks.
- **The user posting to the relay.** The user reads it; they talk to a train
  in its overlay.
- **User-defined agents.** Later.
- **The Investigator and the Watcher in v1.** v1 builds trains with the
  Reviewer as the only one. The other two are defined in a design of their
  own at the end of Order.
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
- **A session of its own in `tracks-back`**, a window in it. The overlay
  attaches a client to that session, so it needs one per train. It is
  never moved into the track's window: today's route, `join-pane` into the
  right column and `break-pane` back, goes (see The Platform).
- **It ends** with the parent, with **End train**, or with
  `tracks train end <name>`. A finished train stays until then, so the
  parent and the user can still ask it.
- **A restart ends trains.** When Tracks or its daemon restarts, the
  track's trains are ended; their messages stay readable.

## The Platform

A narrow pane on the left of the track's window, a small Tracks program
built from the Tracks window's parts: `widget.Frame`, the theme's tokens,
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

- **Shown only while it has something to show:** a train, or a PR with
  open comments. Each frame shows only with its entries: a track with open
  comments and no train has Signals alone. A track with neither has no
  sidebar.
- **No icons** at first. A train's name and agent, its status badge, and for
  the Reviewer its outcome as a second badge, as the Station shows a PR's.
- **Width:** "action required" and the frame need about 22–24 columns. A
  setting hides the sidebar; the track's info then says "2 trains · 1 needs
  you", and the PR buttons go back to the footer's top row.
- **The reviewer moves here, and today's reviewer UI goes:**
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
- **Hide** and `Ctrl+b Esc` close it, as a train's overlay. Opening it
  marks the messages seen by the user; it doesn't mark them read for any
  agent.

### Signals: the PRs' comments

The footer's PR buttons ("PR #12 · solve 3 comments") move into Signals,
one entry per PR with open comments: the PR's label on one line, the
**solve** button under it, so a repo's name (multi-repo tracks) fits the
width.

- They work as now: a click sends the agent the prompt to answer the
  comments; once sent, the entry reads "sent to the agent", muted, until
  new comments come. The click is the user's, so it goes straight in, as
  Collect messages does.
- They keep the variants they take now (1 to 4, in turn).
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
doesn't open its overlay by itself, as the reviewer's pane is shown now.
The train shows `action required` and the user is notified; they open the
overlay to answer.

### The overlay

A click on a train's name opens it over the track: `display-popup` running a
tmux client attached to the train's session. It is the train's own session,
interactive: the user answers its questions, asks it things, scrolls.

```
╭───────────────────────────────────────────────────────────────╮
│ detective · Investigator   active         [ Hide ] [End train] │
│───────────────────────────────────────────────────────────────│
│   (the train's agent)                                         │
╰───────────────────────────────────────────────────────────────╯
```

- **A header row** is the inner client's status line: the train, its badge,
  updated live, and two buttons, ranges handled by the footer's
  `MouseDown1Status` binding.
  - **Hide** detaches the client: the popup closes, the train runs on.
  - **End train** ends it, in `button.variant5`, as Close reviewer now.
- **`Ctrl+b Esc`** hides it too. A bare Esc stays the agent's (it
  interrupts); `Ctrl+b x` would kill the pane. It does nothing outside an
  overlay.
- **No asking a train while it's attached.** A reply to the user would pass
  for the answer; tmux says whether a session has a client.

## The relay

How a track and its trains talk: messages kept in Tracks' database, routed
by Tracks. A relay station more than a chat.

- **Handles.** Each track and train has one, given by Tracks: the track's
  name for the track, the name given at `tracks train new --name`, or the
  agent and a number. Unique within the track.
- **Tracks stamps the sender.** Each agent's environment carries a token, as
  the reviewer's run ID; the daemon maps it to the handle. Nobody sends as
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

- **Its report and answers go on the relay** as @reviewer's, kind `report`
  or `answer`, taken from the hooks as now (Claude's Stop hook; Cursor's
  print-mode turn). The track gets a pointer, as for any message.
- **A question for it** is `tracks review --ask`, or `tracks relay send --to
  reviewer --kind question`. Tracks types the question itself into the
  reviewer's session, behind the fixed lead-in `--ask` uses now: it comes
  from the track, never from another train (the star layout). The delivery
  blockers apply.
- **`tracks review --wait` and `--ask`** read the relay underneath and print
  as now, so the prompts that use them don't change.
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
  whatever blocks it: the user vouches for the input. It stops the retries.
- **Paste and Enter as one:** a bracketed paste (`paste-buffer -p`), then
  Enter, in one tmux call, so keys typed meanwhile don't land inside it.
- **Stall check:** an agent not `active` within about 5 s of the pointer
  didn't take it; the message stays undelivered and the button comes back.
- **Trains get pointers too:** a question from the track to a train is
  delivered the same way, with the same blockers.
- **The user is notified** when a train reports or needs them, through the
  notification channels; not for the track they're looking at.
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
| `tracks train end <name>` | Ends one and closes its pane |
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
4. The relay: the first migration, handles and tokens, `tracks relay
   send|read|show|wait|who`, join and end notices; the Reviewer on it, its
   `--ask` and `--wait` reading it.
5. Delivery: the pointer, the blockers, the tick, the stall check,
   notifications.
6. The Platform: the pane, the list, the badges; the Reviewer in it.
7. The overlay: a session per train in place of a window in
   `tracks-back` (`join-pane` moves a pane out of its session, so the two
   can't come apart), the popup, the header with Hide and End train,
   `Ctrl+b Esc`. Today's reviewer UI goes: the footer's reviewer buttons,
   `join-pane`, the pane shown for the trust question, `button.variant6`.
8. Collect messages.
9. Signals: the Relay Station and its overlay; the PR buttons moved in
   from the footer, the track's info centred again.
10. The track's prompt: the relay paragraph (reading messages when a pointer
    comes).
11. The "started from" note on independent tracks. Independent of the rest.
12. The Investigator and the Watcher: their design (purpose, briefs,
    commands, the Watcher's finish condition, Cursor, the worktree check
    for any train, which runs in the caller's sandbox as the reviewer's
    does: the daemon runs no git), then a step each,
    with `tracks train new`, their Settings frames and the Trains paragraph
    of the track's prompt.

Later: a cap setting, user-defined agents, pinning beside, trains that
write.

Trains talking to each other, if a case needs it, with these rules:

- Only `question` and `answer` between trains; reports and tasks still go
  through the track.
- The track sees every message between trains in its `relay read`, without
  a pointer for them.
- A limit per pair of trains, say 5 messages; after it Tracks refuses, and
  they go through the track.
- Messages stay data, fenced with their sender.

## Open

- Claude on a machine without managed settings: the train switches its
  sandbox on and auto-allows it itself, through `--settings`; to confirm
  in step 12.
- The hosts a command-running train's sandbox lets it reach (`gh`, `curl`).
- The Investigator's and the Watcher's design (step 12).
