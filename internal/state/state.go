// Package state owns the daemon's persistent view of every track that
// has ever been launched. It is intentionally simple: a flat JSON file
// at <state_dir>/state.json, written atomically on every mutation,
// loaded into memory at daemon startup.
//
// "State" here means runtime/operational state (which tracks are
// running, where their worktrees live, what their PIDs are). User
// preferences live in internal/config.
//
// All public Store mutations persist before returning. There's no
// write-behind queue. ~10 concurrent tracks × infrequent state
// transitions is well below the rate where this becomes a problem,
// and write-through saves an entire class of crash-loses-state bugs.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// CurrentSchemaVersion is the SchemaVersion this binary writes. Older
// files on disk are migrated when loaded; newer files are refused so
// a forward-compatible field doesn't get silently dropped.
//
// v2 adds Track.Kind. v3 replaces the single-PR fields (pr_url,
// pr_state, …) with Track.PRs and renames the "pr" status to "pr open".
// v4 adds the top-level State.Proxies list — user-defined stable ports
// and their chosen upstream, previously declared per-service in config
// as proxy_port and held only in the daemon's memory.
// v5 moves the flat review/doc fields (candor, doc_path,
// doc_skip_claim_check, doc_skip_opinion) into Track.Review and
// Track.Doc, and drops Track.LogPath — a path that was computed and
// persisted but never written to or read.
// v6 gives `pr open` a second bit: ExitedAt is now stamped when Claude
// exits on an open PR, which is what separates a track *in review* from
// a live one that opened a PR and kept working (see Track.InReview).
// Code before v6 stamped neither, so a pre-v6 record with the status and
// no exited_at is backfilled from UpdatedAt on load — resolving the
// ambiguity toward review, which is how those records were treated when
// they were written.
// Older tracks are migrated on load (see Track.UnmarshalJSON and
// migrateTrack). A v3 file simply carries no Proxies, which loads as an
// empty list; a v4 file's flat review fields are folded in at decode
// time.
// Three on-disk changes since v5 deliberately did NOT bump this, each
// documented at its field: the observed-model keys were renamed (derived
// data, refolded on decode), RequestedModel was added (absent reads as
// "no preference"), and so was WindowOpen (absent reads as false, which
// only means the track isn't reopened). A bump stops an older binary
// starting at all, which is the heavier cost of the two — worth paying
// here, where the missing field silently misreports a live track as
// finished with, and a finished one as live.
const CurrentSchemaVersion = 6

// Provider is the agent CLI a track runs on. Tracks manages the
// worktree, session and tmux lifecycle identically for every provider;
// the provider decides which binary is launched, how permissions are
// granted, and where its instructions are read from.
type Provider string

const (
	// ProviderClaude runs Claude Code (`claude`). The zero value, so a
	// record written before providers existed reads as Claude — which
	// is what it was.
	ProviderClaude Provider = "claude"

	// ProviderCursor runs Cursor Agent (`agent`), reaching models Claude
	// Code cannot: GPT, Gemini, Grok, Composer.
	ProviderCursor Provider = "cursor"
)

// Providers lists every provider a track may run on, in the order a
// picker should offer them.
func Providers() []Provider { return []Provider{ProviderClaude, ProviderCursor} }

// Valid reports whether p names a provider this binary can launch. The
// empty string is valid and means Claude — see ProviderClaude.
func (p Provider) Valid() bool {
	switch p {
	case "", ProviderClaude, ProviderCursor:
		return true
	}
	return false
}

// Resolved returns the provider to actually launch, mapping the empty
// zero value onto the default rather than making every call site
// remember to.
func (p Provider) Resolved() Provider {
	if p == "" {
		return ProviderClaude
	}
	return p
}

// Label is the provider's display name.
func (p Provider) Label() string {
	switch p.Resolved() {
	case ProviderCursor:
		return "Cursor Agent"
	default:
		return "Claude Code"
	}
}

// Kind is the type of a track. It decides whether the track owns
// worktrees and how Claude is launched.
type Kind string

const (
	// KindWork is the default: a worktree + branch the user edits on.
	KindWork Kind = "work"

	// KindReview is a detached worktree on an existing PR/branch.
	KindReview Kind = "review"

	// KindAsk is a worktree-less, read-only track: Claude points at the
	// primary checkout (in plan permission mode) to answer a question or
	// explore. No branch, no worktree.
	KindAsk Kind = "ask"

	// KindPlan is like KindAsk but framed to produce an implementation
	// plan. Also worktree-less and read-only.
	KindPlan Kind = "plan"

	// KindDoc is a review of a local document (markdown, PDF, image,
	// CSV) rather than a code diff: the target is Track.Doc.Path, not a
	// git ref. Worktree-less — any repos on the track are attached for
	// grounding claims, not for editing. Kept to <=7 chars so it fits
	// the dashboard's KIND column.
	KindDoc Kind = "doc"
)

// Worktreeless reports whether tracks of this kind run without their
// own worktree (pointing at the primary checkout read-only). Such
// tracks skip worktree creation, branch tracking, diff aggregation,
// and worktree removal.
func (k Kind) Worktreeless() bool {
	return k == KindAsk || k == KindPlan || k == KindDoc
}

// Status is the lifecycle phase of a track.
type Status string

const (
	// StatusPending is set briefly between accepting a `tracks new`
	// request and the Claude process being spawned.
	StatusPending Status = "pending"

	// StatusRunning means the Claude process is alive and the log file
	// is growing.
	StatusRunning Status = "running"

	// StatusWaiting means the process is alive but the log file
	// hasn't grown in a while, or a permission prompt is outstanding.
	StatusWaiting Status = "waiting"

	// StatusDone means the Claude process exited cleanly without leaving
	// a merged pull request behind — either it opened none at all, or the
	// ones it opened were closed unmerged.
	StatusDone Status = "done"

	// StatusPROpen means Claude exited after opening at least one pull
	// request that is still open, and the track is deliberately kept
	// alive: review comments, discussion, and follow-up commits are
	// still likely. It is *non-terminal* (see IsTerminal) so the
	// worktree is preserved and token usage keeps accruing. The PR
	// watcher drives it to an end state once every PR is merged/closed;
	// an explicit End/Kill also finalizes it. Renders as "prs open" when
	// the track carries more than one PR (see StatusLabel).
	StatusPROpen Status = "pr open"

	// StatusPRMerged is the end state of a track whose pull requests all
	// landed — the happy path for work that ends in a PR. Terminal and
	// Completed (so a prune sweeps it like Done), but distinct from Done
	// so the dashboard can say the work actually shipped. Renders as
	// "all merged" for a multi-PR track (see StatusLabel).
	StatusPRMerged Status = "pr merged"

	// statusPRLegacy is the pre-v3 spelling of StatusPROpen. Only read
	// during migration (see migrateTrack); never written.
	statusPRLegacy Status = "pr"

	// StatusErrored means the Claude process exited non-zero, or
	// `tracks` was unable to spawn it / set up the worktrees.
	StatusErrored Status = "errored"

	// StatusInterrupted means the track was still live when tracks itself
	// went away — the tmux session was quit, the daemon was stopped, or
	// the machine slept — so Claude was torn down mid-conversation rather
	// than finishing. It is terminal (nothing is running) but, unlike
	// Errored, nothing went wrong: the branch, worktree and Claude
	// session all survive, so the track can be picked back up with
	// `tracks reopen` / `tracks resume`. Deliberately *not* Completed, so a
	// prune-completed sweep never throws away work the user meant to
	// come back to.
	StatusInterrupted Status = "interrupted"

	// StatusDraft is a saved-but-not-launched track: its creation
	// parameters (repos, prompt, slug, …) are persisted in Track.Draft
	// but no worktree exists and Claude was never spawned. Reached when
	// the user saves a failed creation instead of dismissing it, so the
	// entered info survives a fixable problem (e.g. an expired GitHub
	// token). It is *not* terminal — a draft can be launched, which
	// (re)runs creation from its saved parameters.
	StatusDraft Status = "draft"
)

// TrackRepo is one repository participating in a track. The Name
// matches a config.Repo.Name; Path is the absolute path of the
// worktree under <state_dir>/worktrees/<track-id>/<repo-name>.
type TrackRepo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	// Branch is the worktree's current branch as observed by the
	// supervisor. Starts as the daemon's placeholder
	// (`tracks/<id-tail>`); Claude is asked to rename to a
	// conventional `<type>/<slug>` before its first commit, and
	// the next poll picks the new name up.
	Branch string `json:"branch,omitempty"`
}

// Changes is the diff summary the dashboard's detail panel shows for
// the selected track. Summed across all worktrees the track owns, so a
// cross-repo change reads as one figure.
type Changes struct {
	Files      int `json:"files,omitempty"`
	Insertions int `json:"insertions,omitempty"`
	Deletions  int `json:"deletions,omitempty"`
}

// IsZero reports whether this Changes value carries no signal
// (every field is zero). Used by the detail panel to decide whether
// there is a diff worth rendering.
func (c Changes) IsZero() bool {
	return c.Files == 0 && c.Insertions == 0 && c.Deletions == 0
}

// Usage is the token spend + USD cost of a track, summed from Claude
// Code's session transcript by internal/usage. Token counts are the
// *billed* sums across every API call — InputTokens re-counts the
// growing context each turn, which is correct for cost but is not a
// measure of context size.
type Usage struct {
	InputTokens         int64   `json:"input_tokens,omitempty"`
	OutputTokens        int64   `json:"output_tokens,omitempty"`
	CacheReadTokens     int64   `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens int64   `json:"cache_creation_tokens,omitempty"`
	CostUSD             float64 `json:"cost_usd,omitempty"`
}

// IsZero reports whether no usage has been recorded yet.
func (u Usage) IsZero() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 &&
		u.CacheReadTokens == 0 && u.CacheCreationTokens == 0 && u.CostUSD == 0
}

// ServiceStatus is a dev server's lifecycle phase within a track.
type ServiceStatus string

const (
	// ServiceStarting means the process has been started and we're
	// waiting for its readiness probe to pass.
	ServiceStarting ServiceStatus = "starting"
	// ServiceReady means the readiness probe passed (or there was none
	// and post-start hooks have run) — the service is usable.
	ServiceReady ServiceStatus = "ready"
	// ServiceRunning means the process is up but has no readiness probe,
	// so we can't assert it's serving yet.
	ServiceRunning ServiceStatus = "running"
	// ServiceFailed means the process exited non-zero, never started, or
	// failed a hook / readiness wait.
	ServiceFailed ServiceStatus = "failed"
	// ServiceStopped means the process was torn down (by us or the track).
	ServiceStopped ServiceStatus = "stopped"
)

// Live reports whether a service in this status is expected to be
// serving. Every pre-terminal status (starting, running, ready) counts;
// failed/stopped do not. This is a question about the service, not about
// its process — for teardown, ask ServiceState.NeedsTeardown.
func (s ServiceStatus) Live() bool {
	switch s {
	case ServiceStarting, ServiceRunning, ServiceReady:
		return true
	default:
		return false
	}
}

// ServiceState records one running (or finished) dev server for a track.
// PGID is the process-group id used to tear the whole tree down with a
// single signal — it's the authoritative handle, persisted so teardown
// works even after a daemon restart.
type ServiceState struct {
	Name      string        `json:"name"`
	Status    ServiceStatus `json:"status"`
	PID       int           `json:"pid,omitempty"`
	PGID      int           `json:"pgid,omitempty"`
	Port      int           `json:"port,omitempty"`
	LogPath   string        `json:"log_path,omitempty"`
	StartedAt *time.Time    `json:"started_at,omitempty"`
	ExitedAt  *time.Time    `json:"exited_at,omitempty"`
}

// Active reports whether the service still occupies its name from the
// user's point of view: it is live, or it failed its readiness probe but
// still holds the pane process (and therefore the port). This is the
// predicate the UI and the "is it already running?" checks want — a
// failed-but-still-running service is exactly the one the user needs to
// see and act on, and Live() alone hides it.
func (s ServiceState) Active() bool {
	return s.Status.Live() || s.NeedsTeardown()
}

// NeedsTeardown reports whether this service still has a process group
// that teardown must signal. Every status except Stopped counts —
// Failed included, because a service that failed its readiness probe
// usually still has a live pane process holding its port. Skipping it
// would leak that process and its port past the end of the track.
func (s ServiceState) NeedsTeardown() bool {
	return s.Status != ServiceStopped && s.PGID > 0
}

// PRRef is one pull request a track opened. Tracks routinely produce
// more than one — a stack of PRs, or a follow-up alongside the main
// change — so each is recorded separately and the track's status is a
// roll-up over all of them.
type PRRef struct {
	// URL is the marker value the daemon saw in the track's pane
	// (TRACKS_PR_URL=<url>). It's the identity of the entry.
	URL string `json:"url"`

	// State / Draft / ReviewState / Comments are filled by the track's
	// gh-poll goroutine. Empty until its first poll lands.
	State       string `json:"state,omitempty"` // OPEN / CLOSED / MERGED
	Draft       bool   `json:"draft,omitempty"`
	ReviewState string `json:"review_state,omitempty"` // APPROVED / CHANGES_REQUESTED / REVIEW_REQUIRED
	Comments    int    `json:"comments,omitempty"`
}

// Open reports whether this PR is still awaiting a merge/close decision.
// A PR we haven't polled yet (empty State) counts as open — the watcher
// corrects it on its first poll.
func (p PRRef) Open() bool { return p.State != "MERGED" && p.State != "CLOSED" }

// Merged reports whether this PR landed.
func (p PRRef) Merged() bool { return p.State == "MERGED" }

// Number is the "#123" shorthand parsed off the tail of the PR URL, or
// "" when the URL doesn't end in a number. Used to tell PRs apart in
// notifications on a track carrying several.
func (p PRRef) Number() string {
	i := strings.LastIndex(p.URL, "/")
	if i < 0 || i == len(p.URL)-1 {
		return ""
	}
	tail := p.URL[i+1:]
	for _, r := range tail {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return "#" + tail
}

// Track is the persistent record of one Claude session.
type Track struct {
	// ID is opaque to the user: <YYYYMMDD-HHMMSS>-<6char-rand>.
	// Used for filesystem paths and tmux window naming.
	ID string `json:"id"`

	// Branch is the <type>/<slug> branch created in every worktree.
	Branch string `json:"branch"`

	// Slug is an optional human label the user typed at track
	// creation time. Independent of the branch name (Claude picks
	// that). Shown in the dashboard so several tracks against the
	// same repo are easy to tell apart. Empty when the user left
	// the field blank.
	Slug string `json:"slug,omitempty"`

	// Window is the tmux window name this track owns, chosen once at
	// creation and never recomputed. Persisting it is what lets the name
	// be a bare human label: uniqueness is settled once, against what
	// already exists, instead of being guaranteed on every call by
	// stapling the track id on. Empty on tracks created before this
	// existed — see WindowName, which falls back to the old derived form
	// so their windows stay reachable.
	//
	// Added without a schema bump: an older binary ignores the key and
	// falls back to that derived name, which carries the id tail and so
	// is unique. The downgrade fails safe as "window not found" rather
	// than targeting somebody else's window.
	Window string `json:"window,omitempty"`

	// WindowOpen records that tracks opened a window for this track and
	// the track has not been closed since. It is the "the user still has
	// this one open" bit that Status cannot carry: a done or pr-merged
	// track keeps its window until the track is closed, and one kept open
	// is usually one the user means to keep working in — a merged PR
	// followed by more work on the same topic, without re-supplying the
	// context.
	//
	// Set when a window is opened (spawnSupervisor) and cleared when the
	// track is closed (endTrack), when a promote's re-spawn fails, and
	// when a restart finds the track orphaned with its Claude still
	// running. A respawn that kills the window and then fails to open
	// another keeps the flag on purpose: nobody closed that track, and it
	// should be offered again. Deliberately not derived from tmux:
	// the daemon's own shutdown is usually *triggered* by the session
	// going away, so at the moment the question is asked there is nothing
	// left to ask — and that is also why the flag survives a crash or a
	// machine restart, which is exactly when the reopen set matters.
	//
	// It follows that the pane is not the handle: typing `exit` in a
	// finished track's shell does not take it out of the set, because it
	// does not close the track — the record, the worktree and the
	// dashboard row are all still there. Closing it does.
	//
	// Added without a schema bump: absent reads as false, so an older
	// binary simply doesn't reopen the track, and a track spawned once
	// under this version gains the flag.
	WindowOpen bool `json:"window_open,omitempty"`

	// Kind is the track type (work/review/ask/plan/doc). Empty in v1
	// files; migrated to KindWork on load. Drives worktree handling and
	// how Claude is launched.
	Kind Kind `json:"kind,omitempty"`

	// Review carries the settings that only mean something when the track
	// is reviewing something — code or a document. Non-nil on KindReview
	// and KindDoc, nil on every other kind, so the nil check *is* the
	// "is this a review?" question and there are no dead settings to
	// inherit across a promotion. Migrated records are held to the same
	// rule — see ensureReviewSpec.
	//
	// Replace the pointer, never write through it: Store.Get and All hand
	// out shallow struct copies, so a spec is shared with the stored track
	// and with every snapshot a reader is holding. Same hazard AddPR and
	// SetPR copy-on-write around.
	Review *ReviewSpec `json:"review,omitempty"`

	// Doc describes the document under review. Non-nil on KindDoc only.
	// A doc track carries both this and Review: a document review *is* a
	// review (it has a candor level) and additionally has a target and
	// section switches. Replace the pointer, never write through it — see
	// the note on Review.
	Doc *DocSpec `json:"doc,omitempty"`

	// Repos lists the participating worktrees, in the order they were
	// added (initial selection first, mid-session add-repo calls
	// appended).
	Repos []TrackRepo `json:"repos"`

	// Ports maps a declared service name to the TCP port reserved for it
	// in this track. Allocated once at track creation (arithmetic only —
	// nothing is bound) and kept clear of other live tracks' ports. Empty
	// when the track's repos declare no services.
	Ports map[string]int `json:"ports,omitempty"`

	// Services records the dev servers started for this track (lazy, via
	// `tracks up`). Each entry carries the process-group id used to tear
	// it down. Empty until a service is started.
	Services []ServiceState `json:"services,omitempty"`

	// Status is the most recently observed lifecycle phase.
	Status Status `json:"status"`

	// PID of the Claude process. Zero before spawn, retained after
	// exit so post-mortems can correlate.
	PID int `json:"pid,omitempty"`

	// TaskPrompt is the prompt the user typed. Stored so the dashboard
	// can show it without re-reading the log.
	TaskPrompt string `json:"task_prompt"`

	// PRs are the pull requests this track opened, in the order their
	// TRACKS_PR_URL=<url> markers first appeared in the pane. Empty
	// until the daemon sees one. Pre-v3 tracks carried a single PR in
	// flat pr_* fields; those are folded into PRs[0] on load.
	PRs []PRRef `json:"prs,omitempty"`

	// LastOutput is a freshly-captured snippet of the bottom of the
	// track's tmux pane — the last few non-empty lines after ANSI
	// escapes are stripped. Used by the dashboard to surface what
	// Claude is currently doing (or what question it's waiting on)
	// without the user having to switch windows.
	LastOutput string `json:"last_output,omitempty"`

	// AwaitingInput is true when the supervisor detected a Claude
	// confirmation/choice block in the pane (the `☐ ` marker plus a
	// numbered option list). In that state LastOutput holds the
	// full prompt — question + options — so the dashboard can
	// render it as the highlight, not just an arbitrary tail.
	AwaitingInput bool `json:"awaiting_input,omitempty"`

	// Changes is the diff summary (files / insertions / deletions)
	// between the track's branch and its base, plus uncommitted
	// edits in the worktree. Refreshed by the supervisor every
	// poll. Zero values mean nothing produced yet or the worktree
	// is gone.
	Changes Changes `json:"changes,omitempty"`

	// SessionID is the UUID passed to `claude --session-id` at spawn.
	// Lets the daemon find this track's transcript under
	// ~/.claude/projects/*/<SessionID>.jsonl to total token usage.
	SessionID string `json:"session_id,omitempty"`

	// Usage is the token spend + cost, refreshed by the supervisor
	// from the session transcript. Zero until the first assistant
	// turn lands.
	Usage Usage `json:"usage,omitempty"`

	// ObservedModel is the model id of the track's most recent
	// main-chain assistant turn, read from the same transcript as Usage.
	// It is what actually ran — it follows a `/model` switch inside the
	// pane without tracks being told, and so is the model to price and
	// display. Deliberately ignores sub-agent turns. Empty until the
	// first turn.
	//
	// "Observed" distinguishes it from a model the user *asked* for at
	// creation; the two can disagree, and when they do this one is the
	// truth.
	//
	// Added without a schema bump, unlike the fields above: it is
	// *derived*, not authoritative. An older binary drops the key on its
	// next write and a newer one re-derives it from the transcript on the
	// next refresh, so a downgrade round-trip is self-healing and there is
	// nothing for a migration to preserve.
	//
	// The rename from "model" is nonetheless handled in UnmarshalJSON
	// rather than left to re-derivation: a track that already finished
	// never refreshes again, so it would have kept an empty cell for
	// good.
	ObservedModel string `json:"observed_model,omitempty"`

	// ObservedSubagentModel is the model of the track's most recent
	// sub-agent turn, read from the same transcript. Kept apart from
	// ObservedModel because the pair is the point: a track can run Opus
	// itself while its reviewer subagent runs Haiku. Derived and
	// schema-exempt for the same reason. Empty until a sub-agent takes a
	// turn.
	ObservedSubagentModel string `json:"observed_subagent_model,omitempty"`

	// RequestedModel is the model explicitly picked when the track was
	// created, passed to the CLI as --model.
	//
	// Empty means the user expressed no preference — NOT that no model
	// applies. The configured default for the track's kind is resolved
	// at spawn time instead (see claude.BuildOptions), deliberately
	// rather than being baked in here: promote flips an ask/plan track
	// to KindWork and re-spawns it, and a default frozen at creation
	// would run the promoted work session on the ask model.
	//
	// Kept for the two paths that re-spawn rather than resume:
	// promoting, and relaunching a draft. Both start a fresh session, so
	// without this an explicit pick would silently revert to the
	// default. Resume needs nothing from it — a resumed session restores
	// its own model, and deliberately keeps a mid-session `/model`
	// switch.
	//
	// Diverges from ObservedModel whenever the user switches models in
	// the pane, and when a name the CLI doesn't recognise quietly
	// resolves to a different model. ObservedModel is the truth; this is
	// the intent.
	//
	// Added without a schema bump, unlike Review/Doc. This one is
	// authoritative rather than derived, so an older binary drops it on
	// write and the choice is lost. A bump is still the worse trade: a
	// v5 binary refuses to load a store written at v6 (see
	// FileStore.load) and so won't start at all until the file is put
	// back — the tracks survive, but access to all of them doesn't,
	// which beats losing one optional field on each.
	//
	// Note this is a different case from the v5 bump, which moved
	// authoritative fields that could not be reconstructed if dropped.
	// An absent RequestedModel is indistinguishable from "no preference",
	// which is a valid state with a sensible behaviour.
	RequestedModel string `json:"requested_model,omitempty"`

	// Provider is the agent CLI this track runs on, chosen at creation.
	// Empty means Claude Code — see ProviderClaude — so every record
	// written before providers existed reads correctly without a
	// migration.
	//
	// Fixed for the life of the track. Changing it would orphan the
	// session: SessionID is a Claude session uuid or a Cursor chat id,
	// and neither binary can resume the other's.
	//
	// Added without a schema bump, for the reason given at
	// RequestedModel: a v6 store is refused outright by a v5 binary, so
	// a bump costs the user access to every track rather than one
	// optional field on each.
	Provider Provider `json:"provider,omitempty"`

	// CreatedAt is when the track entry was written.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the last time any field on this track changed.
	UpdatedAt time.Time `json:"updated_at"`

	// ExitedAt is when the track's Claude process stopped: set on every
	// end state, on an interruption, and on the transition into review
	// (see InReview) — a track sitting on an open PR has no Claude
	// behind it either. Cleared again when a resume spawns a new one.
	ExitedAt *time.Time `json:"exited_at,omitempty"`

	// ExitCode is the Claude process's exit code if available.
	ExitCode *int `json:"exit_code,omitempty"`

	// ErrorMsg is a human-readable reason the track is in
	// StatusErrored — a failed git fetch, a spawn error, or an
	// orphaned-by-restart note. Empty for tracks that never errored.
	// Surfaced in the dashboard so a failed track explains itself
	// without digging through the daemon log. On a StatusDraft track it
	// holds the reason the last creation attempt failed; on a
	// StatusInterrupted one, what tore the track down (a deliberate quit
	// vs. an unclean daemon exit).
	ErrorMsg string `json:"error_msg,omitempty"`

	// Draft holds the parameters needed to (re)create this track. It is
	// captured whenever creation fails so the attempt can be saved as a
	// draft and launched later without re-entering everything. Non-nil
	// on StatusDraft tracks and on failed-creation StatusErrored tracks;
	// nil once a track has been successfully created.
	Draft *DraftSpec `json:"draft,omitempty"`
}

// ReviewSpec is how a review is delivered. Shared by code reviews
// (KindReview) and document reviews (KindDoc).
type ReviewSpec struct {
	// Candor dials the *delivery* of the review: 1 is radical candor, 10
	// is honest but gently framed. Zero means the user didn't pick one —
	// read it through Track.CandorLevel(), which supplies DefaultCandor.
	// Never affects which findings a review reports or their severity;
	// see claude.docReviewBrief and claude.reviewCandorSuffix for how it
	// reaches the reviewer.
	Candor int `json:"candor,omitempty"`
}

// DocSpec is the document a KindDoc track reviews.
type DocSpec struct {
	// Path is the absolute path of the document — a file, or a directory
	// of files. Its parent directory is passed to Claude as an --add-dir
	// so the file is readable (documents usually live outside every
	// configured repo).
	Path string `json:"path"`

	// SkipClaimCheck / SkipOpinion drop one of the optional sections of
	// the review. Stored as negations so the zero value keeps both on.
	SkipClaimCheck bool `json:"skip_claim_check,omitempty"`
	SkipOpinion    bool `json:"skip_opinion,omitempty"`
}

// DocPath is the document under review, or "" when the track has none.
// Nil-safe, so callers don't have to know whether Doc is set.
func (t Track) DocPath() string {
	if t.Doc == nil {
		return ""
	}
	return t.Doc.Path
}

// SkipClaimCheck / SkipOpinion report whether the corresponding optional
// section of a doc review is switched off. Both are false for a track
// with no document, which is the right default: nothing is skipped.
func (t Track) SkipClaimCheck() bool { return t.Doc != nil && t.Doc.SkipClaimCheck }
func (t Track) SkipOpinion() bool    { return t.Doc != nil && t.Doc.SkipOpinion }

// DraftSpec is the set of user-supplied parameters that a track is
// created from. Persisted on a track (see Track.Draft) so a creation
// that failed — or was deliberately saved before launch — can be
// relaunched from exactly what the user entered. Mirrors the daemon's
// NewParams; kept in the state package so it can live on Track without
// state importing the daemon package.
type DraftSpec struct {
	Repos             []string `json:"repos,omitempty"`
	TaskPrompt        string   `json:"task_prompt,omitempty"`
	Slug              string   `json:"slug,omitempty"`
	ReviewRef         string   `json:"review_ref,omitempty"`
	DocPath           string   `json:"doc_path,omitempty"`
	Kind              string   `json:"kind,omitempty"`
	Candor            int      `json:"candor,omitempty"`
	DocSkipClaimCheck bool     `json:"doc_skip_claim_check,omitempty"`
	DocSkipOpinion    bool     `json:"doc_skip_opinion,omitempty"`

	// Model is the model picked at creation, so a relaunch runs the same
	// one rather than the current default.
	Model string `json:"model,omitempty"`

	// Provider is the agent CLI picked at creation, so a relaunch runs
	// the same one rather than the current default.
	Provider Provider `json:"provider,omitempty"`
}

// IsTerminal reports whether s is one of the end-state statuses —
// nothing is running and no supervisor owns the track.
func (s Status) IsTerminal() bool {
	return s == StatusDone || s == StatusPRMerged ||
		s == StatusErrored || s == StatusInterrupted
}

// Completed reports whether the track reached an end state of its own
// accord (Claude finished, or something failed) rather than being cut
// short by a tracks shutdown. Prune-completed uses this so interrupted
// tracks — which the user still intends to reopen — are never swept up
// alongside genuinely finished ones.
func (s Status) Completed() bool {
	return s == StatusDone || s == StatusPRMerged || s == StatusErrored
}

// StatusLabel is the track's status as shown to the user. It differs
// from the raw status only for the two PR statuses, which pluralize
// over how many PRs the track opened: one reads "pr open" / "pr
// merged", several read "prs open" / "all merged". Every renderer
// (dashboard, `tracks ls`, the menu pickers) goes through this so they
// can't disagree about what a track's status is called.
func (t Track) StatusLabel() string {
	switch t.Status {
	case StatusPROpen:
		if len(t.PRs) > 1 {
			return "prs open"
		}
	case StatusPRMerged:
		if len(t.PRs) > 1 {
			return "all merged"
		}
	}
	return string(t.Status)
}

// OpenPRs counts the track's pull requests that haven't been merged or
// closed yet.
func (t Track) OpenPRs() int {
	n := 0
	for _, p := range t.PRs {
		if p.Open() {
			n++
		}
	}
	return n
}

// MergedPRs counts the track's pull requests that landed.
func (t Track) MergedPRs() int {
	n := 0
	for _, p := range t.PRs {
		if p.Merged() {
			n++
		}
	}
	return n
}

// HasOpenPR reports whether the track is still waiting on a decision
// for at least one of its pull requests. Such a track is kept in review
// rather than finalized.
func (t Track) HasOpenPR() bool { return t.OpenPRs() > 0 }

// AllPRsMerged reports whether the track opened at least one pull
// request and every one of them merged. This is what earns a track
// StatusPRMerged instead of StatusDone.
func (t Track) AllPRsMerged() bool {
	return len(t.PRs) > 0 && t.MergedPRs() == len(t.PRs)
}

// PRIndex returns the index of the PR with the given URL in t.PRs, or
// -1 when the track doesn't know that URL.
func (t Track) PRIndex(url string) int {
	for i, p := range t.PRs {
		if p.URL == url {
			return i
		}
	}
	return -1
}

// AddPR appends url as a new pull request unless the track already
// knows it. Reports whether anything was added.
//
// Copy-on-write, like SetPR: a Track handed out by the store shares its
// PRs backing array with the stored one, so appending in place could
// write into a snapshot another goroutine is still reading.
func (t *Track) AddPR(url string) bool {
	if url == "" || t.PRIndex(url) >= 0 {
		return false
	}
	prs := make([]PRRef, len(t.PRs), len(t.PRs)+1)
	copy(prs, t.PRs)
	t.PRs = append(prs, PRRef{URL: url})
	return true
}

// SetPR replaces the pull request at index i, and reports whether that
// changed anything. Out-of-range indices are a no-op.
//
// The slice is copied before the write: Store.Update hands the mutator a
// *struct* copy, which still shares the PRs backing array with the
// stored track and with any snapshot a reader (e.g. a `tracks ls` about
// to be serialized) took earlier. Writing an element in place would be
// visible to those readers without synchronisation.
func (t *Track) SetPR(i int, p PRRef) bool {
	if i < 0 || i >= len(t.PRs) || t.PRs[i] == p {
		return false
	}
	prs := make([]PRRef, len(t.PRs))
	copy(prs, t.PRs)
	prs[i] = p
	t.PRs = prs
	return true
}

// InReview reports whether the track is sitting on an open pull request
// with its Claude already gone — the state enterPRReview leaves behind.
//
// StatusPROpen alone does not say that. A live session that opens PR #1
// and keeps working (the stacked-PR flow) is moved to the same status by
// nextLiveStatus, and that track still has Claude in its window. ExitedAt
// is what separates the two: it is stamped when the process stops.
//
// Treating the status as proof of review was how a live track could
// disappear across a restart — the shutdown sweep skipped it as "already
// settled" and startup re-adopted it as a PR watch with no window, so it
// was never offered for reopen.
func (t Track) InReview() bool {
	return t.Status == StatusPROpen && t.ExitedAt != nil
}

// Dormant reports whether the track has no Claude process behind it, so
// a fresh one can be spawned on its session: every end state, plus a
// track in review. A live track is not dormant (attach to its window
// instead), and neither is a draft, which was never spawned at all.
func (t Track) Dormant() bool {
	return t.Status.IsTerminal() || t.InReview()
}

// ShouldReopen reports whether tracks should bring this track back when
// it next starts: the user still had it open (see WindowOpen) and there
// is no Claude behind it to attach to instead.
//
// What the track was *doing* is deliberately not part of the test. It is
// in the set because it was on screen when tracks stopped — a pr-merged
// track kept open for the next round of work on the same topic comes
// back the same way a running one does. Closing it (`tracks done`, or
// the dashboard's own close) is what takes it back out.
//
// An interrupted track is included whatever the flag says, so records
// written before WindowOpen existed keep the behaviour they had.
//
// The session id is not checked here. A track that predates session
// tracking has nothing to resume, but dropping it silently would leave
// the user wondering where it went; it stays in the set and handleReopen
// reports it as a per-track failure that says exactly why.
func (t Track) ShouldReopen() bool {
	return t.Dormant() && (t.WindowOpen || t.Status == StatusInterrupted)
}

// Resumable reports whether the track's Claude conversation can be
// picked up again with `claude --resume`: nothing may be running behind
// it (see Dormant) and it must carry the session UUID the transcript is
// stored under.
func (t Track) Resumable() bool {
	return t.Dormant() && t.SessionID != ""
}

// CanLaunch reports whether the track can be (re)created from saved
// parameters — i.e. it carries a Draft spec and isn't currently active.
// True for a saved draft and for a failed-creation errored track.
func (t Track) CanLaunch() bool {
	return t.Draft != nil && (t.Status == StatusDraft || t.Status.IsTerminal())
}

// Duration is the track's wall-clock runtime: from CreatedAt to
// ExitedAt for a finished track, or to now for a live one. Zero when
// CreatedAt isn't set.
func (t Track) Duration() time.Duration {
	if t.CreatedAt.IsZero() {
		return 0
	}
	end := time.Now().UTC()
	if t.ExitedAt != nil {
		end = *t.ExitedAt
	}
	return end.Sub(t.CreatedAt)
}

// windowLabelMaxLen caps the human part of a tmux window name so the
// status-bar tab stays readable. Raised from 24 once the id suffix
// stopped being appended: names that read "swap-reset-after-multi-s"
// were losing their last word to a suffix nobody read.
const windowLabelMaxLen = 32

// legacyWindowLabelMaxLen is frozen at the old value and must stay
// there. It only feeds legacyWindowName, which has to reproduce — byte
// for byte — the name a pre-Window track's window was actually opened
// under. Widening it would silently repoint every one of those tracks
// at a window that does not exist.
const legacyWindowLabelMaxLen = 24

// DocDir returns the directory Claude needs access to in order to read
// the track's document: the path itself when it's a directory, its
// parent when it's a file. Empty when the track has no document.
//
// Falls back to the parent when the path can't be stat'd — a document
// deleted between track creation and a later resume shouldn't break
// spawning; Claude reports the missing file instead.
func (t Track) DocDir() string {
	path := t.DocPath()
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return path
	}
	return filepath.Dir(path)
}

// Candor bounds. The scale runs 1 (radical candor) to 10 (honest but
// gently framed); DefaultCandor leans blunt deliberately — a review
// that reads as optional is a review the reader skips.
const (
	MinCandor     = 1
	MaxCandor     = 10
	DefaultCandor = 3
)

// CandorLevel is the track's review candor, normalised. Zero (the user
// never picked one, or the track predates the setting) and any
// out-of-range value fall back to DefaultCandor.
func (t Track) CandorLevel() int {
	if t.Review == nil || t.Review.Candor < MinCandor || t.Review.Candor > MaxCandor {
		return DefaultCandor
	}
	return t.Review.Candor
}

// candorLabels is the one-phrase gloss for each level. Lives here so the
// new-track picker and the prompt the daemon renders for Claude describe
// the same scale — and so a level still reads unambiguously in the
// prompt even if the reviewer agent definition is stale or missing.
//
// The band names (radical candor / direct / measured / diplomatic /
// gently framed) are the ones the reviewer agents use for the same pairs
// of levels, so a user who picks 6 here and reads the agent definition
// finds their choice described in the same words.
var candorLabels = [MaxCandor + 1]string{
	1:  "radical candor — lead with the problem, no cushioning",
	2:  "radical candor — blunt, with minimal framing",
	3:  "direct — plain statements, no hedging",
	4:  "direct — states the problem, adds the why",
	5:  "measured — neutral and even-handed",
	6:  "measured — findings posed as shared problems",
	7:  "diplomatic — leads with what works, findings as suggestions",
	8:  "diplomatic — soft framing, problems posed as questions",
	9:  "gently framed — heavily cushioned, nothing stated flatly",
	10: "gently framed — maximally kind wording, still nothing omitted",
}

// CandorLabel describes a candor level in one phrase. Out-of-range
// levels get DefaultCandor's label, matching CandorLevel.
func CandorLabel(level int) string {
	if level < MinCandor || level > MaxCandor {
		level = DefaultCandor
	}
	return candorLabels[level]
}

// WindowName is the tmux window name for this track. It's the single
// source of truth: the daemon opens the window under this name and
// every selector/killer (CLI, dashboard, supervisor) targets it by the
// same name, so they must all agree.
//
// Normally that's Window, chosen once at creation (see
// Server.claimWindowName) and stored — a bare human label like
// "swap-rate-tooltip", with a "-2" appended only if something already
// answered to the plain form.
//
// A track created before Window existed has none, and falls back to the
// name it was actually opened under: <label>-<id-tail>, where the id
// tail was stapled on unconditionally to keep two tracks sharing a slug
// from colliding — which would have made the daemon kill or select the
// wrong window. Those windows keep their old names for life; only new
// tracks get clean ones.
func (t Track) WindowName() string {
	if t.Window != "" {
		return t.Window
	}
	return t.legacyWindowName()
}

// legacyWindowName is the pre-Window derived form, kept so tracks that
// predate the field still resolve to the window they were opened under.
func (t Track) legacyWindowName() string {
	suffix := t.ID
	if len(t.ID) > 6 {
		suffix = t.ID[len(t.ID)-6:]
	}
	label := windowLabelCapped(t.Slug, legacyWindowLabelMaxLen)
	if label == "" {
		label = windowLabelCapped(t.TaskPrompt, legacyWindowLabelMaxLen)
	}
	if label == "" {
		return "t-" + suffix
	}
	return label + "-" + suffix
}

// LegacyFallbackWindowName is the id-suffixed form, exported so the
// daemon can fall back to it for a track with no usable label, or when
// every variant of a label is already spoken for. Unique by
// construction, at the cost of being unreadable.
func (t Track) LegacyFallbackWindowName() string { return t.legacyWindowName() }

// WindowLabel is the human part of a window name for a track: the
// user's slug if they set one, otherwise the opening words of the task
// prompt, otherwise "" — the caller decides what to do with a track
// that offers no usable text (see Server.claimWindowName).
func (t Track) WindowLabel() string {
	if l := windowLabel(t.Slug); l != "" {
		return l
	}
	return windowLabel(t.TaskPrompt)
}

// windowLabel slugifies s into a tmux-safe token: lowercase ASCII
// alphanumerics, with every other run collapsed to a single hyphen.
// This deliberately strips ":" and "." (tmux target separators) and
// whitespace (which would break the status-bar tab). The result is
// truncated at maxLen so a long prompt doesn't produce a giant tab.
// The cut is by length, not on a word boundary, so a label can end
// mid-word ("investigate-the-rate-spike-on-sw").
// Returns "" when s carries no usable characters.
func windowLabel(s string) string { return windowLabelCapped(s, windowLabelMaxLen) }

func windowLabelCapped(s string, maxLen int) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(s) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		switch {
		case isAlnum:
			if b.Len() >= maxLen {
				// Already at the cap; stop at this word boundary.
				return strings.TrimRight(b.String(), "-")
			}
			b.WriteRune(r)
			prevHyphen = false
		case !prevHyphen && b.Len() > 0:
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// State is the entire on-disk payload.
type State struct {
	SchemaVersion int            `json:"schema_version"`
	Tracks        []Track        `json:"tracks"`
	Proxies       []ProxyBinding `json:"proxies,omitempty"`
}

// ProxyBinding is one user-defined stable port and, optionally, the
// upstream it currently forwards to. It is the persisted form of a
// proxy: the daemon materialises a live listener from it and re-applies
// the upstream on restart when the target server is still running.
//
// Keyed by PublicPort. An empty upstream (UpstreamTrackID == "") means
// the port is free — the proxy returns 503 and the port is released, so
// an idle daemon holds no proxy ports.
type ProxyBinding struct {
	// PublicPort is the fixed port the app points at (e.g. 3000).
	PublicPort int `json:"public_port"`

	// BindAll listens on every interface instead of loopback. Off by
	// default; turn it on for a port a physical device must reach.
	BindAll bool `json:"bind_all,omitempty"`

	// UpstreamTrackID / UpstreamService identify the running dev server
	// this port forwards to. Both empty means free. The target may be a
	// service of any name on any track — the binding is not tied to a
	// service name the way the old per-service proxy_port was.
	UpstreamTrackID string `json:"upstream_track_id,omitempty"`
	UpstreamService string `json:"upstream_service,omitempty"`
}

// Store is the interface the daemon uses to talk to persistent state.
// Implementations: FileStore (real) and MemoryStore (tests).
type Store interface {
	// All returns a snapshot of every known track, sorted by CreatedAt
	// ascending. The returned slice is owned by the caller and safe
	// to mutate.
	All() []Track

	// Get fetches a single track by ID.
	Get(id string) (Track, bool)

	// Put inserts or updates a track. UpdatedAt is set automatically
	// to time.Now().UTC().
	Put(t Track) error

	// Update atomically read-modify-writes a single track under the
	// store's own lock, so a concurrent writer can't land between the
	// read and the write and clobber a field the caller didn't touch
	// (the lost-update a separate Get+Put pair is prone to). mutate
	// receives a pointer to the stored track and reports whether it
	// changed anything worth persisting. Returns the resulting track and
	// whether the track existed; an unknown id is (zero, false, nil) and
	// mutate is not called.
	Update(id string, mutate func(*Track) bool) (Track, bool, error)

	// Delete removes a track. Returns false if it didn't exist.
	Delete(id string) (bool, error)

	// AllProxies returns a snapshot of every proxy binding, sorted by
	// PublicPort ascending. The returned slice is owned by the caller.
	AllProxies() []ProxyBinding

	// GetProxy returns the binding for a port, if defined.
	GetProxy(port int) (ProxyBinding, bool)

	// PutProxy inserts or updates a binding, keyed by PublicPort, and
	// persists. A PublicPort of zero is rejected.
	PutProxy(b ProxyBinding) error

	// DeleteProxy removes the binding for a port. Returns false if none
	// existed.
	DeleteProxy(port int) (bool, error)

	// UpdateProxy atomically read-modify-writes the binding for a port
	// under the store's own lock. mutate reports whether it changed
	// anything worth persisting. An unknown port is (zero, false, nil)
	// and mutate is not called.
	UpdateProxy(port int, mutate func(*ProxyBinding) bool) (ProxyBinding, bool, error)
}

// FileStore is a Store backed by <state_dir>/state.json.
//
// All access is serialized by an RWMutex. Mutations are written to a
// temp file and renamed into place so a partial write can never
// corrupt the canonical file.
type FileStore struct {
	path string

	mu      sync.RWMutex
	tracks  map[string]Track
	proxies map[int]ProxyBinding
}

// OpenFileStore loads (or creates) the state file at
// <stateDir>/state.json and returns a ready-to-use FileStore. Missing
// file → empty store. Parse errors are surfaced — the user should
// know if their state file is unreadable.
func OpenFileStore(stateDir string) (*FileStore, error) {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	fs := &FileStore{
		path:    filepath.Join(stateDir, "state.json"),
		tracks:  make(map[string]Track),
		proxies: make(map[int]ProxyBinding),
	}
	if err := fs.load(); err != nil {
		return nil, err
	}
	return fs, nil
}

func (fs *FileStore) load() error {
	data, err := os.ReadFile(fs.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read %s: %w", fs.path, err)
	}
	if len(data) == 0 {
		return nil
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("parse %s: %w", fs.path, err)
	}
	if s.SchemaVersion > CurrentSchemaVersion {
		return fmt.Errorf("%s: schema_version %d newer than supported (%d)",
			fs.path, s.SchemaVersion, CurrentSchemaVersion)
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for _, t := range s.Tracks {
		migrateTrack(&t, s.SchemaVersion)
		fs.tracks[t.ID] = t
	}
	for _, b := range s.Proxies {
		if b.PublicPort > 0 {
			fs.proxies[b.PublicPort] = b
		}
	}
	return nil
}

// migrateTrack upgrades a track loaded from an older schema in place.
// from is the file's SchemaVersion, needed by any migration that would
// be wrong to re-apply to a record this binary wrote itself.
//
// v1 had no Kind; infer it from the branch (pr/* came from review
// tracks) and default everything else to work. v2 spelled the
// in-review status "pr"; it's "pr open" from v3 on. (The v2 flat pr_*
// fields are folded into PRs by UnmarshalJSON, which has to run at
// decode time to see them at all.) v5 and earlier never stamped
// ExitedAt on entering review — see the backfill below, which is
// version-gated precisely because a v6 record with the same shape means
// the opposite thing.
func migrateTrack(t *Track, from int) {
	if t.Kind == "" {
		if strings.HasPrefix(t.Branch, "pr/") {
			t.Kind = KindReview
		} else {
			t.Kind = KindWork
		}
	}
	if t.Status == statusPRLegacy {
		t.Status = StatusPROpen
	}
	// Pre-v6, `pr open` with no exit stamp was the only shape a track in
	// review could have. From v6 it means the opposite — a live session
	// that opened a PR and kept working — so this must never run against
	// a file this binary wrote, or the next restart would decide a live
	// track had finished. UpdatedAt is the closest thing on record to
	// when Claude stopped; only Duration reads the value, and the whole
	// point is the field being set.
	if from < 6 && t.Status == StatusPROpen && t.ExitedAt == nil {
		stamped := t.UpdatedAt
		if stamped.IsZero() {
			// A pre-v2 record can carry neither timestamp, leaving a zero
			// stamp. Deliberate: what the migration owes the track is a set
			// field, and the only reader is Duration, which already
			// short-circuits on a zero CreatedAt.
			stamped = t.CreatedAt
		}
		t.ExitedAt = &stamped
	}
	// Kind is only just settled for a pre-v2 record, so the review-spec
	// invariant has to be re-checked now it's known.
	t.ensureReviewSpec()
}

// UnmarshalJSON decodes a Track, folding two generations of removed
// fields into their replacements:
//
//   - pre-v3: the single-PR fields (pr_url, pr_state, …) become PRs[0].
//   - pre-v5: the flat review/doc fields (candor, doc_path,
//     doc_skip_claim_check, doc_skip_opinion) become Review and Doc.
//   - the renamed observed-model keys (model, subagent_model) become
//     ObservedModel and ObservedSubagentModel. No schema bump went with
//     that rename — the fields are derived, so the only thing at stake
//     is a finished track's display, which this fold preserves.
//
// Done here rather than in migrateTrack because none of those fields
// exists on Track any more — decode time is the only place they are
// still visible. Tracks decoded from the daemon socket get the same
// treatment, so an older state file needs no rewrite before it can be
// served.
func (t *Track) UnmarshalJSON(data []byte) error {
	type track Track // shed the method set to avoid recursing
	var aux struct {
		track
		LegacyPRURL         string `json:"pr_url"`
		LegacyPRState       string `json:"pr_state"`
		LegacyPRDraft       bool   `json:"pr_draft"`
		LegacyPRReviewState string `json:"pr_review_state"`
		LegacyPRComments    int    `json:"pr_comments"`

		LegacyCandor         int    `json:"candor"`
		LegacyDocPath        string `json:"doc_path"`
		LegacyDocSkipClaim   bool   `json:"doc_skip_claim_check"`
		LegacyDocSkipOpinion bool   `json:"doc_skip_opinion"`

		LegacyModel         string `json:"model"`
		LegacySubagentModel string `json:"subagent_model"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*t = Track(aux.track)
	if len(t.PRs) == 0 && aux.LegacyPRURL != "" {
		t.PRs = []PRRef{{
			URL:         aux.LegacyPRURL,
			State:       aux.LegacyPRState,
			Draft:       aux.LegacyPRDraft,
			ReviewState: aux.LegacyPRReviewState,
			Comments:    aux.LegacyPRComments,
		}}
	}
	// A v4 file carries the review settings flat. Only fold them in when
	// the new field is absent, so a v5 file that legitimately has both
	// (it never should) isn't overwritten by stale keys.
	if t.Review == nil && aux.LegacyCandor != 0 {
		t.Review = &ReviewSpec{Candor: aux.LegacyCandor}
	}
	if t.Doc == nil && aux.LegacyDocPath != "" {
		t.Doc = &DocSpec{
			Path:           aux.LegacyDocPath,
			SkipClaimCheck: aux.LegacyDocSkipClaim,
			SkipOpinion:    aux.LegacyDocSkipOpinion,
		}
	}
	// The observed-model keys were renamed. Both are derived, so a live
	// track would re-derive them on its next refresh — but a track that
	// already reached a terminal status never refreshes again
	// (finalizeTrack returns early on one), and would lose its model
	// display permanently. Carry the old keys across instead.
	if t.ObservedModel == "" {
		t.ObservedModel = aux.LegacyModel
	}
	if t.ObservedSubagentModel == "" {
		t.ObservedSubagentModel = aux.LegacySubagentModel
	}
	t.ensureReviewSpec()
	return nil
}

// ensureReviewSpec gives a review or doc track an empty ReviewSpec when
// it hasn't got one, so "Review != nil" really does answer "is this
// reviewing something?" for migrated records too.
//
// Without it a record written before the candor dial existed — or any
// v4 record that simply omitted the key — decodes as a review-kind track
// with a nil Review, and the invariant the struct promises is only true
// for tracks created from v5 on. An empty spec is the right filler:
// CandorLevel() reads it as DefaultCandor, which is exactly what those
// tracks resolved to before.
//
// Called from UnmarshalJSON (where Kind is whatever the file said) and
// again from migrateTrack (which runs later and infers Kind for pre-v2
// records that never had one).
func (t *Track) ensureReviewSpec() {
	if t.Review != nil {
		return
	}
	if t.Kind == KindReview || t.Kind == KindDoc {
		t.Review = &ReviewSpec{}
	}
}

// Path returns the absolute path of the state file (useful for
// debugging and tests).
func (fs *FileStore) Path() string { return fs.path }

// All returns a snapshot sorted by CreatedAt ascending.
func (fs *FileStore) All() []Track {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	out := make([]Track, 0, len(fs.tracks))
	for _, t := range fs.tracks {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

// Get returns the track with the given ID, if any.
func (fs *FileStore) Get(id string) (Track, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	t, ok := fs.tracks[id]
	return t, ok
}

// Put inserts or updates a track and flushes to disk.
func (fs *FileStore) Put(t Track) error {
	if t.ID == "" {
		return errors.New("Track.ID must not be empty")
	}
	t.UpdatedAt = time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = t.UpdatedAt
	}
	fs.mu.Lock()
	fs.tracks[t.ID] = t
	err := fs.flushLocked()
	fs.mu.Unlock()
	return err
}

// Update read-modify-writes a track atomically under fs.mu and flushes
// to disk when mutate reports a change. See Store.Update.
func (fs *FileStore) Update(id string, mutate func(*Track) bool) (Track, bool, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	t, ok := fs.tracks[id]
	if !ok {
		return Track{}, false, nil
	}
	if mutate(&t) {
		t.UpdatedAt = time.Now().UTC()
		if t.CreatedAt.IsZero() {
			t.CreatedAt = t.UpdatedAt
		}
		fs.tracks[id] = t
		if err := fs.flushLocked(); err != nil {
			return t, true, err
		}
	}
	return t, true, nil
}

// Delete removes a track and flushes to disk. Returns whether the
// track existed.
func (fs *FileStore) Delete(id string) (bool, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if _, ok := fs.tracks[id]; !ok {
		return false, nil
	}
	delete(fs.tracks, id)
	if err := fs.flushLocked(); err != nil {
		return true, err
	}
	return true, nil
}

// AllProxies returns every binding, sorted by PublicPort ascending.
func (fs *FileStore) AllProxies() []ProxyBinding {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return sortedProxies(fs.proxies)
}

// GetProxy returns the binding for a port, if defined.
func (fs *FileStore) GetProxy(port int) (ProxyBinding, bool) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	b, ok := fs.proxies[port]
	return b, ok
}

// PutProxy upserts a binding by PublicPort and flushes to disk.
func (fs *FileStore) PutProxy(b ProxyBinding) error {
	if b.PublicPort <= 0 {
		return errors.New("ProxyBinding.PublicPort must be positive")
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.proxies[b.PublicPort] = b
	return fs.flushLocked()
}

// DeleteProxy removes a binding and flushes. Returns whether it existed.
func (fs *FileStore) DeleteProxy(port int) (bool, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if _, ok := fs.proxies[port]; !ok {
		return false, nil
	}
	delete(fs.proxies, port)
	if err := fs.flushLocked(); err != nil {
		return true, err
	}
	return true, nil
}

// UpdateProxy read-modify-writes a binding atomically under fs.mu and
// flushes when mutate reports a change. See Store.UpdateProxy.
func (fs *FileStore) UpdateProxy(port int, mutate func(*ProxyBinding) bool) (ProxyBinding, bool, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	b, ok := fs.proxies[port]
	if !ok {
		return ProxyBinding{}, false, nil
	}
	if mutate(&b) {
		fs.proxies[port] = b
		if err := fs.flushLocked(); err != nil {
			return b, true, err
		}
	}
	return b, true, nil
}

// sortedProxies returns the bindings ordered by PublicPort ascending so
// the on-disk file and every snapshot are stable.
func sortedProxies(m map[int]ProxyBinding) []ProxyBinding {
	out := make([]ProxyBinding, 0, len(m))
	for _, b := range m {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublicPort < out[j].PublicPort })
	return out
}

// flushLocked writes the current in-memory state to disk atomically.
// Caller must hold fs.mu.Lock().
func (fs *FileStore) flushLocked() error {
	tracks := make([]Track, 0, len(fs.tracks))
	for _, t := range fs.tracks {
		tracks = append(tracks, t)
	}
	sort.Slice(tracks, func(i, j int) bool {
		return tracks[i].CreatedAt.Before(tracks[j].CreatedAt)
	})
	payload := State{
		SchemaVersion: CurrentSchemaVersion,
		Tracks:        tracks,
		Proxies:       sortedProxies(fs.proxies),
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(fs.path)
	tmp, err := os.CreateTemp(dir, ".state.*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), fs.path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// MemoryStore is an in-process Store for tests. It implements the
// same interface as FileStore but never touches disk.
type MemoryStore struct {
	mu      sync.RWMutex
	tracks  map[string]Track
	proxies map[int]ProxyBinding
}

// NewMemoryStore returns an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tracks:  make(map[string]Track),
		proxies: make(map[int]ProxyBinding),
	}
}

func (m *MemoryStore) All() []Track {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Track, 0, len(m.tracks))
	for _, t := range m.tracks {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (m *MemoryStore) Get(id string) (Track, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tracks[id]
	return t, ok
}

func (m *MemoryStore) Put(t Track) error {
	if t.ID == "" {
		return errors.New("Track.ID must not be empty")
	}
	t.UpdatedAt = time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = t.UpdatedAt
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tracks[t.ID] = t
	return nil
}

// Update read-modify-writes a track atomically under m.mu. See Store.Update.
func (m *MemoryStore) Update(id string, mutate func(*Track) bool) (Track, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tracks[id]
	if !ok {
		return Track{}, false, nil
	}
	if mutate(&t) {
		t.UpdatedAt = time.Now().UTC()
		if t.CreatedAt.IsZero() {
			t.CreatedAt = t.UpdatedAt
		}
		m.tracks[id] = t
	}
	return t, true, nil
}

func (m *MemoryStore) Delete(id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tracks[id]; !ok {
		return false, nil
	}
	delete(m.tracks, id)
	return true, nil
}

func (m *MemoryStore) AllProxies() []ProxyBinding {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return sortedProxies(m.proxies)
}

func (m *MemoryStore) GetProxy(port int) (ProxyBinding, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	b, ok := m.proxies[port]
	return b, ok
}

func (m *MemoryStore) PutProxy(b ProxyBinding) error {
	if b.PublicPort <= 0 {
		return errors.New("ProxyBinding.PublicPort must be positive")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proxies[b.PublicPort] = b
	return nil
}

func (m *MemoryStore) DeleteProxy(port int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.proxies[port]; !ok {
		return false, nil
	}
	delete(m.proxies, port)
	return true, nil
}

func (m *MemoryStore) UpdateProxy(port int, mutate func(*ProxyBinding) bool) (ProxyBinding, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.proxies[port]
	if !ok {
		return ProxyBinding{}, false, nil
	}
	if mutate(&b) {
		m.proxies[port] = b
	}
	return b, true, nil
}

// Compile-time interface checks.
var (
	_ Store = (*FileStore)(nil)
	_ Store = (*MemoryStore)(nil)
)
