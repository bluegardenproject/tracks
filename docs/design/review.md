# Reviews

**Status:** being built, one PR per step under Order. Steps 1 to 9 are
built. The reviewer pane, its button and showing it for the trust question
give way to the Platform and the overlay of [trains.md](trains.md).

## Problem

- **The reviewer is tied to the track's engine.** A Claude track reviews with
  the in-session `tracks-reviewer` subagent, on Claude's model. A second
  opinion from another engine and model (a Cursor reviewer on a Claude track,
  or the other way round) isn't possible.
- **A Review track's agent does everything.** It takes the user's brief,
  resolves links, runs the review, presents it and answers follow-ups, all on
  one model. The review needs the strongest model; the rest doesn't.
- **Reports flag extreme edge cases.** Warns, and even blocks, describe
  scenarios that need inputs or states the code around them rules out. They
  look as serious as real defects, and sorting them out is the user's work.
- **Large PRs.** In a big monorepo a PR can touch hundreds of files across
  several teams. The user is responsible for their team's part only.

## Not in scope

- **Posting to GitHub.** Reviews never comment on a PR. The user reads the
  report in Tracks and decides what to raise in the PR themselves. Posting
  would need agreement with the teams first, so PRs don't fill up with
  generated comments.
- **Cost of reviewer sessions.** Per-track cost is Claude-only and inaccurate,
  and may be removed; reviewer sessions don't add to it.

## What exists

| | Claude track | Cursor track |
|---|---|---|
| Pre-push review | `tracks-reviewer` subagent via the Task tool | `tracks review`: a headless `agent -p` with the diff |
| Review track | The same subagent | The same `tracks review` |
| Reviewer's model | The session's | `engines.cursor.model` |
| Instructions | `claude.ReviewerInstructions()` | The same text |
| Guards | — | `TRACKS_REVIEWING` marker and a per-track lock |

`tracks review` already holds most of what an engine-independent reviewer
needs: one set of instructions, the diff built by Tracks, the base from the
track's repo, the candor level, the guards.

## Roles

| Role | Model | Does |
|---|---|---|
| Orchestrator | The track's own, can be a smaller one | Takes the user's brief, resolves links it can open (a ticket, a doc), writes the brief, runs `tracks review`, presents the report, answers questions the report already answers |
| Reviewer | The Reviewer agent's (Settings → Trains (agents)) | Reviews in a session of its own, in the reviewer pane; answers questions about its review |

The orchestrator never reviews itself; when `tracks review` fails it says so
and stops. On a Work track the working agent is the orchestrator for its
pre-push review.

### What each may do

| | Edit files | Commit | Push, open a PR | Show a fix in the console |
|---|---|---|---|---|
| Reviewer, any track | never | never | never | in the report |
| Orchestrator, Review track | never | never | never | yes, as a suggestion |
| Orchestrator, Work track | yes | yes | after the pre-push review; a PR only once the user agreed, as a draft where the repo's Draft PRs setting says so | yes |

- **The reviewer can't change anything.** It runs in its engine's read-only
  mode (see The reviewer pane).
- **Tracks checks it.** `tracks review` notes the worktree's HEAD, `git
  status`, the changes to tracked files and the untracked files' contents
  before the reviewer starts, and compares them after. A change is
  reported above the findings, as an error, never quietly kept.
- **The orchestrator follows its track's kind.** The prompts carry the rules
  above: a Review track's ends with its read-only contract; a Work track's
  asks the user before opening a PR.

## Reviewer setting

- Settings has an **Agents** section, after Tracks, for the agents Tracks
  starts beside a track's own. Each has a frame of its own; **Reviewer** is
  the first, with Agent and Model fields, stored as `agents.reviewer` in
  `settings.yaml`. Its agent can be *Same as the track*, the default; a model
  counts only with an agent.
- It picks the agent of the reviewer pane (below). Until orchestrators use
  the pane (step 5), the setting applies where `tracks review` runs: Cursor
  tracks; Claude tracks keep their subagent.
- A per-track override in the New track form comes later, if a global setting
  turns out not to be enough. It needs a migration.

## The reviewer pane

The reviewer is an interactive session of the Reviewer agent and model, in a
pane of its own, hidden until the user opens it.

- **Started by Tracks, not the orchestrator.** `tracks review` asks the daemon,
  which opens the pane through tmux. Its process is tmux's, not the
  orchestrator's: the orchestrator's sandbox doesn't apply, and no new way
  around it opens either. The reviewer runs as any track's agent does, with
  its engine's own permissions and sandbox for its commands.
- **Hidden.** Hidden panes live in a background tmux session of Tracks', a
  window per track, so they never show among the tracks.
- **Read-only.** Claude starts in plan mode with `Edit`, `Write` and
  `NotebookEdit` denied; Cursor with `--mode ask`. `tracks review` also
  checks the worktree before and after, in the orchestrator's own sandbox.
- **The report comes back to Tracks**, and `tracks review` waits for it and
  prints it to the orchestrator; the reviewer never runs a command.
  - **Claude:** its `Stop` hook carries its reply. Its brief comes in with
    `--append-system-prompt-file`, which Claude Code reads itself: a folder
    given with `--add-dir` makes it ask, in the hidden pane, whether to trust
    it, on every review.
  - **Cursor:** its interactive CLI's hooks carry no reply text, and their
    transcript path is empty (checked live). So the review is one turn in
    print mode, which the pane hands to Tracks; the same chat then goes on
    interactively, for questions. The turn streams (`--output-format
    stream-json`) through `tracks review-stream`, which shows its tool calls
    and text as they come, so the pane isn't blank for the minutes a review
    takes. It hands over the text after the turn's last tool call, as the
    Claude reviewer's last message is, or for a review the last such text
    with the outcome line.
  - A reply that has the `REVIEW OUTCOME:` line is the report. A run ID in
    the reviewer's hook command keeps a reply of an ended reviewer from
    counting for the new one.
- **Questions it asks wait for the user.** A Claude reviewer on a worktree
  Claude hasn't worked in, such as a Cursor track's, asks whether to trust it.
  Claude asks as it starts, so the footer's tick reads a hidden Claude
  reviewer's screen in its review's first minute only, and when the question
  is on it, shows the pane for the user to answer: once a review, so a pane
  hidden again stays hidden.
- **In Work tracks too.** The pane opens in the Work track's window and
  worktree, so a pre-push review sees what isn't committed yet. The pane
  route replaced the in-session `tracks-reviewer` subagent, which Tracks no
  longer installs and removes where it installed it.

### The button

While a track has a reviewer, the footer's top row shows a green
**Open reviewer** button. It moves the hidden pane into the track's right
column, as a dev-server pane sits, running on as it was (`join-pane`); the
button becomes **Hide reviewer**, which moves it back. Beside it, a magenta
**Close reviewer** ends the reviewer, closes its pane and drops its run, so a
`tracks review --wait` on it stops. The track's info (name · status ·
engine) stays on the row's left, beside the buttons: the reviewer's buttons
last as long as the reviewer, often the track, so they can't take the
info's place as the comment buttons did.

Two button variants are added. They aren't part of the rotation the footer's
other buttons take (variants 1 to 4); each belongs to one button:

| Variant | Use | Default (dark) | Default light |
|---|---|---|---|
| `button.variant5` | Close reviewer | bg `#700c50`, text `#ffb5e7` | bg `#ffd9f1`, text `#700c50` |
| `button.variant6` | Open reviewer / Hide reviewer | bg `#00916f`, text `#c9fff0` | bg `#c9fff0`, text `#00664f` |

### Lifetime

- **One reviewer per track.** It stays, hidden or shown, until the track ends
  or the user clicks **Close reviewer**.
- **A new review starts fresh.** `tracks review` ends a reviewer that's there
  and starts a new session, after the branch changed or for another scope.
- **Closing its pane ends it**, such as the orchestrator running
  `tracks close` when the user asks.
- **Interaction is limited.** The reviewer answers questions about its review
  and the code it reviewed. It takes no new tasks, doesn't plan or build, and
  points anything else back to the orchestrator: it is not a second track.

### Why not the earlier routes

- **Headless from the orchestrator's shell** (built in step 1): a Claude
  track's sandbox keeps the reviewer's CLI from its network.
- **Headless from the daemon:** the daemon runs outside any sandbox, so the
  reviewer did too. Claude's could be locked down with flags; Cursor's needed
  an OS sandbox and still loaded hooks and config from the worktree, which the
  agent can write. Dropped.

## The review packet

Tracks builds the packet; the orchestrator adds only what needs judgment or a
connection Tracks doesn't have. Code is always local: a Review track's
worktree is detached at the PR head or the branch tip, so GitHub adds what's
around the code, never the code.

| Part | Pre-push | PR review | From |
|---|---|---|---|
| Diff | `base...HEAD`, and what isn't committed | `origin/<PR base>...HEAD` | git |
| Changed files | Paths with added and removed lines | The same | git |
| Intent | Commit messages | PR title and body | git, `gh` |
| Discussion | — | Open review threads, so findings aren't repeated and addressed ones are checked | `gh` |
| Checks | — | CI results; a failing check is a finding, its log the evidence | `gh pr checks` |
| Brief | The user's words | The form's prompt, and what the orchestrator resolved | `--brief <file>` |
| Settings | Candor, base, scope | The same | Tracks |

- **From GitHub, by the daemon.** The PR's parts come through `gh` with the
  PR's URL only, never in a worktree, so no repo's config is read and the
  agent's sandbox needs no network. What can't be fetched is named in the
  brief, for the report's Not run line, and the review goes on without it.
  Failed checks bring the end of their failed steps' log (`gh run view
  --log-failed`), for up to three checks.
- **A moved PR.** Before a PR review, Tracks compares the worktree's HEAD with
  the PR's head SHA. When the PR moved on, `tracks review` refuses and says
  how to fetch the new head, for the orchestrator to ask the user;
  `--stale` reviews what's there, and the brief says it's an older head. It
  never reviews a stale head silently.
- **Limits.** The changed-file list holds up to 3,000 files, and says how
  many more there are. Diffs over the limit
  aren't cut in the middle of a file: whole files are left out, the largest
  first, and listed as not reviewed in the report's Not run line. Threads and
  CI logs get their own limits.

## Checks

Review tracks don't run lint, typecheck or tests locally. A PR review relies on
the CI results in the packet; a failing check is a finding. A Review track's
setup stays lazy, as now.

## Scope: a code owner's part

`tracks review --owner <handle>`, and a **Code owner** field per repo on the
Repositories tab that sets it by default (empty: the whole diff). The
default applies to Review tracks only: a Work track's pre-push review checks
its own whole change. `--all` reviews the whole change despite the field.

- Tracks reads the repo's `CODEOWNERS` (`.github/`, the root, `docs/`, the
  first found) on the base branch, as GitHub does, so a branch can't change
  its own scope. GitHub's rules: the last matching pattern wins, a name also
  covers what's in a folder of that name, a trailing `/*` matches only the
  files directly in its folder, and lines with `!` or `[ ]` are skipped.
  Parsing and matching are a pure function with tests. An owner is matched
  without regard to case, and a handle may be given without its `@`.
- The packet's diff holds only the files `<handle>` owns, and the PR's
  threads only those on them. The other changed files are listed, marked
  outside the scope, without diffs. The reviewer may still read any file
  for context, such as a caller outside the scope.
- The report's first line says the scope: `Reviewed 38 of 412 changed files,
  those owned by <handle>.` A change outside the scope that breaks owned code
  is still reported, under `### Whole diff`.
- No `CODEOWNERS`, or no file owned by the handle: the review stops and says
  so, rather than reviewing everything.

## Likelihood and confidence

Each block and warn gets two numbers next to its severity; hints, style
and naming points, carry none:

```
- 🔴 **block** · `L42` · 90% sure · 60% likely — <one-sentence finding>
  > Why: …
  > Path: <the caller chain or input that gets there>
```

- **Confidence** (*sure*): how certain the reviewer is that the defect is
  real. It is high when the reviewer read the code involved and traced the
  path; low when the finding rests on code it didn't open or behaviour it
  assumes.
- **Likelihood** (*likely*): how probable the failure scenario is in real use,
  given the callers and data that exist today. The reviewer anchors it:

  | Likely | Means |
  |---|---|
  | 70–100% | A normal path: common inputs, every run, every user |
  | 30–70% | Realistic conditions: a slow network, an empty list, a retry |
  | 5–30% | Valid but unusual inputs or timing |
  | 0–5% | Needs a state the callers or types rule out today |

- **Path.** A block or warn names how the scenario is reached: the call chain
  or input. No path found: likelihood 20% at most, and the report says the
  path wasn't found.
- Severity stays what it is now, how bad the failure is when it happens. The
  numbers sit beside it, so the user can sort out edge cases at a glance.

### Experiments

How well the numbers work is measured, not assumed. In order:

1. **Report only.** The numbers in the report; severity unchanged. The user
   sees whether they match their judgement.
2. **A verify pass.** Each block and warn goes to a second reviewer (another
   model, or the same with a fresh context) told to refute it. A finding it
   refutes is dropped; one it confirms keeps its confidence. Costs a call per
   finding; on for blocks only at first.
3. **Gating.** If the numbers prove reliable, severity follows them: a block
   under 10% likely becomes a warn, a warn under 5% a hint. Thresholds come
   from step 4.
4. **Calibration.** In Tracks the user marks a finding *raised* (flagged in the
   PR) or *dismissed*. Stored per finding with its numbers, which shows over
   time whether 60% likely means anything. Needs the findings stored, so the
   report becomes structured data (JSON next to the text).

## Follow-ups

The orchestrator answers what the report already holds: "summarise the
blocks", "which findings touch the cache?", "explain finding 3". It doesn't
add findings or change their severity, likelihood or confidence.

Everything that needs the code read again goes back to the reviewer:

- **A question about a finding** ("is finding 3 a false positive?", "look
  deeper at the cache"): the user asks in the reviewer pane, or the
  orchestrator runs `tracks review --ask "<question>"`, which types it into
  the reviewer's session and returns its answer. The reviewer's first reply
  after the question is the answer; `tracks review --wait` then waits for it
  instead of the report. One question at a time, only once the report is
  in, and only while the pane is hidden: a user who shows it talks to the
  reviewer there, and a reply of theirs would pass for the answer. The
  question goes in behind a fixed lead-in, on one line, so it can't switch
  Claude's input to a shell (`!`) or slash command. Cursor's hooks don't
  carry its replies in the interactive CLI, so a Cursor reviewer's pane is
  started again (`respawn-pane`) on a print-mode turn on the question, in
  the same chat, and goes on interactively after it: what was typed in the
  pane but not sent is lost.
- **A new review**, after the branch changed, or **another scope** (another
  `--owner`, the whole diff): a new `tracks review`, a new session.

## Control surface

| Command | Does |
|---|---|
| `tracks review` | Starts a reviewer session on the track's branch, ending one that's there, and prints its report |
| `--brief <file>` | Adds the orchestrator's brief to the packet |
| `--owner <handle>` | Reviews only the files `<handle>` owns |
| `--all` | Reviews the whole change, despite the repo's Code owner |
| `--ask <question>` | Asks the reviewer that's there, and prints its answer |
| `--wait` | Waits for the report of the review that's running, or its answer to the last question |
| `--base <ref>` | As now |
| `--stale` | Reviews a PR's worktree that's behind the PR |

## Order

Each step is a PR of its own, branched from `main` where it can be, so they
can be reviewed and merged while the next is built.


1. `tracks review` engine-independent, and the Claude reviewer's flags.
   Built.
2. Settings → Agents → Reviewer: the reviewer's agent and model. Built.
3. The reviewer pane: the hidden pane, read-only modes, the report through
   the hooks, `tracks review` waiting for it, a new review ending the old
   session.
4. The footer button, Open and Hide reviewer, with `button.variant6`;
   `button.variant5` added and reserved.
5. Orchestrators use the pane: Work and Review tracks, both engines; the
   `tracks-reviewer` subagent goes.
6. `tracks review --ask`.
7. The packet: changed files, PR intent, threads, checks, the moved-PR check,
   per-file limits. Built.
8. Code-owner scope: `CODEOWNERS` matching, `--owner`, the repo field.
   Built.
9. Likelihood and confidence in the report (experiment 1). Built: on blocks
   and warns, with a Path line; hints carry none.
10. The verify pass (experiment 2).
11. Structured findings and calibration (experiments 3 and 4).

## Open

- Where calibration data is shown: per engine and model, to compare reviewers.
