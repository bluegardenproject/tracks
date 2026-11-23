# Trains

**Status:** agreed in discussion, not built. One PR per step under Order.

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
- **User-defined agents.** Later; the three below come first.
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

| Agent | For | May | Reports |
|---|---|---|---|
| **Reviewer** | Reviewing the diff, answering questions on it | Read; no commands | `REVIEW OUTCOME:` through the hooks, as now |
| **Investigator** | A failing test or CI job, research in the code, pulling data, reproducing a bug, a second opinion on a plan | Read, run commands; never edit | One report through the relay |
| **Watcher** | CI, PR comments, a dev server's logs | Read, run commands; never edit; runs long | An event through the relay each time something happens; a report when its condition is met |

- **Settings → Trains (agents)** is today's Settings → Agents, renamed. A
  frame per agent, each with Agent and Model, as the Reviewer's. The keys
  stay under `agents` in `settings.yaml` (`agents.reviewer`,
  `agents.investigator`, `agents.watcher`), so existing settings keep
  working.
- **Each agent has a fixed brief**, golden-checked; the parent's task is
  added to it, marked as the task.
- `tracks review` stays the Reviewer's command. It is a train on the
  Platform, but keeps its route through the hooks until a later step moves
  it to the relay.

### Never edits

"Read only" can't mean plan mode for the Investigator and the Watcher: they
run tests, `gh`, `curl`, read logs.

- Edit tools are denied, as the reviewer's are.
- Tracks checks the worktree before and after, as it does for a review:
  tracked files, and untracked files that aren't ignored. Build output that
  is ignored is fine. A change is an `error` on the train and in its report.

### Commands it may run

A train in a hidden pane that asks "allow this command?" for each command
would sit in `action required` most of its time.

- **Each agent has a fixed set of allowed commands**, in its brief and its
  engine's flags: `git` that only reads, `gh`, the repo's test runners,
  `curl`. The spike finds how far Claude's permission flags go.
- **Anything else is asked as now:** the train is `action required`, the
  user is notified and answers in the overlay.
- **Cursor:** ask mode runs no commands (`agents/cursor/reviewer.go`), and
  the other modes edit. The spike looks for a Cursor permission config that
  allows the commands and denies edits. Without one, the Investigator and
  the Watcher run on Claude only in v1, and their Settings frames say so.

## Lifetime

- **Started by Tracks, not the parent.** `tracks train new` asks the
  daemon, which opens the train's pane through tmux, as for the reviewer:
  the parent's sandbox doesn't apply, and no way around it opens.
- **A session of its own in `tracks-back`**, a window in it. The overlay
  attaches a client to that session, so it needs one per train.
- **It ends** with the parent, with **End train**, or with
  `tracks train end <name>`. A finished Investigator stays until then, so
  the parent and the user can still ask it.
- **A Watcher's finish condition is in its task**, with a time limit, two
  hours unless the task says otherwise. It reports when the condition is
  met or the time is up.
- **A restart ends trains.** When Tracks or its daemon restarts, the
  track's trains are ended; their messages stay readable.

## The Platform

A narrow pane on the left of the track's window, a small Tracks program
built from the Tracks window's parts: `widget.Frame`, the theme's tokens,
the Station's status badges. It holds two frames: **Platform**, the trains,
and below it **Signals**, the Relay Station.

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
╰───────────────────────────╯
```

- **Shown only while the track has a train.** It opens with the first and
  closes with the last; a track without trains looks as it does now.
- **No icons** at first. A train's name and agent, its status badge, and for
  the Reviewer its outcome as a second badge, as the Station shows a PR's.
- **Width:** "action required" and the frame need about 22–24 columns. A
  setting hides the Platform; the track's info then says "2 trains · 1 needs
  you".
- **The reviewer moves here.** The footer's Open/Hide and Close reviewer
  buttons go; the top row keeps the track's info and the PR buttons.
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
  notice: "@detective exited without a report".

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

If the spike finds the input line can't be read reliably on an engine,
that engine's agents go without the last check: not typing, and Collect
messages, cover it.

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
| `tracks train new <agent> --task <text>` | Starts a train; `--name` sets its handle, `--task-file -` reads the task |
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

## Order

Each step a PR of its own.

0. **Spike**, not merged: the reviewer in an overlay. Keys and mouse in a
   nested client on Tracks' socket; Claude and Cursor redrawing at popup
   size; the header row; `client_activity` changing on keys only; reading
   an empty input line on both engines; how far Claude's permission flags
   let a train run its commands unasked; a Cursor permission config that
   runs commands but doesn't edit.
1. `badge()` to `ui/widget`.
2. Settings → Trains (agents): the rename, the Investigator and Watcher
   frames.
3. Trains: the migration, the daemon's trains, a session per train in
   `tracks-back`, `tracks train new|list|end`, the briefs, the worktree
   check. Ending with the parent.
4. The relay: the migration, handles and tokens, `tracks relay
   send|read|show|wait|who`, join and end notices.
5. Delivery: the pointer, the blockers, the tick, the stall check,
   notifications.
6. The Platform: the pane, the list, the badges; the Reviewer in it, its
   footer buttons gone.
7. The overlay: the popup, the header with Hide and End train,
   `Ctrl+b Esc`.
8. Collect messages.
9. Signals: the Relay Station and its overlay.
10. The Watcher: events, its finish condition and time limit.
11. The track's prompt: the Trains paragraph.
12. The "started from" note on independent tracks. Independent of the rest.

Later: the Reviewer on the relay (its `--ask` and `--wait` with it), a cap
setting, user-defined agents, pinning beside, trains that write.

Trains talking to each other, if a case needs it, with these rules:

- Only `question` and `answer` between trains; reports and tasks still go
  through the track.
- The track sees every message between trains in its `relay read`, without
  a pointer for them.
- A limit per pair of trains, say 5 messages; after it Tracks refuses, and
  they go through the track.
- Messages stay data, fenced with their sender.

## Open

Nothing for v1 beyond what the spike (step 0) settles: the allowed
commands on each engine, and the input-line check.
