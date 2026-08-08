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
// Older tracks are migrated on load (see Track.UnmarshalJSON and
// migrateTrack).
const CurrentSchemaVersion = 3

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
	// CSV) rather than a code diff: the target is Track.DocPath, not a
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

// Changes is the diff summary the dashboard shows in the CHANGES
// column. Summed across all worktrees the track owns, so a
// cross-repo change reads as one row in the dashboard.
type Changes struct {
	Files      int `json:"files,omitempty"`
	Insertions int `json:"insertions,omitempty"`
	Deletions  int `json:"deletions,omitempty"`
}

// IsZero reports whether this Changes value carries no signal
// (every field is zero). Used by the dashboard to decide whether
// to render the CHANGES column for a track.
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

	// Kind is the track type (work/review/ask/plan/doc). Empty in v1
	// files; migrated to KindWork on load. Drives worktree handling and
	// how Claude is launched.
	Kind Kind `json:"kind,omitempty"`

	// DocPath is the absolute path of the document under review on a
	// KindDoc track — a file, or a directory of files. Its parent
	// directory is passed to Claude as an --add-dir so the file is
	// readable (documents usually live outside every configured repo).
	// Empty on every other kind.
	DocPath string `json:"doc_path,omitempty"`

	// Candor dials the *delivery* of a review on KindReview / KindDoc
	// tracks: 1 is radical candor, 10 is honest but gently framed. Zero
	// means the user didn't pick one — read it through CandorLevel(),
	// which supplies DefaultCandor. Never affects which findings a review
	// reports or their severity; see claude.docReviewBrief and
	// claude.reviewCandorSuffix for how it reaches the reviewer.
	Candor int `json:"candor,omitempty"`

	// DocSkipClaimCheck / DocSkipOpinion drop one of the optional
	// sections of a doc review. Stored as negations so the zero value —
	// and therefore every track written before these existed — keeps
	// both sections on.
	DocSkipClaimCheck bool `json:"doc_skip_claim_check,omitempty"`
	DocSkipOpinion    bool `json:"doc_skip_opinion,omitempty"`

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

	// LogPath is the absolute path to the stream-json log file. Useful
	// post-mortem.
	LogPath string `json:"log_path"`

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

	// CreatedAt is when the track entry was written.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the last time any field on this track changed.
	UpdatedAt time.Time `json:"updated_at"`

	// ExitedAt is set once Status reaches Done or Errored.
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

// Resumable reports whether the track's Claude conversation can be
// picked up again with `claude --resume`: it must be in an end state
// (nothing running) and must carry the session UUID the transcript is
// stored under.
func (t Track) Resumable() bool {
	return t.Status.IsTerminal() && t.SessionID != ""
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
// status bar tab stays readable. The unique ID suffix is appended on
// top of this.
const windowLabelMaxLen = 24

// DocDir returns the directory Claude needs access to in order to read
// the track's document: DocPath itself when it's a directory, its
// parent when it's a file. Empty when the track has no DocPath.
//
// Falls back to the parent when the path can't be stat'd — a document
// deleted between track creation and a later resume shouldn't break
// spawning; Claude reports the missing file instead.
func (t Track) DocDir() string {
	if t.DocPath == "" {
		return ""
	}
	if info, err := os.Stat(t.DocPath); err == nil && info.IsDir() {
		return t.DocPath
	}
	return filepath.Dir(t.DocPath)
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
	if t.Candor < MinCandor || t.Candor > MaxCandor {
		return DefaultCandor
	}
	return t.Candor
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
// every selector/killer (CLI, dashboard, supervisor) targets it by
// the same name, so they must all agree.
//
// The name reads as <label>-<id-tail>:
//
//   - <label> is a slugified human hint — the user's Slug if they set
//     one, otherwise the opening words of the task prompt — so the tab
//     in tmux's status bar means something at a glance.
//   - <id-tail> is the trailing 6 characters of the track ID, always
//     appended so two tracks sharing a slug never collide on a name
//     (which would make the daemon kill or select the wrong window).
//
// When there's no usable label (no slug, empty prompt) it falls back
// to the historical "t-<id-tail>" form.
func (t Track) WindowName() string {
	suffix := t.ID
	if len(t.ID) > 6 {
		suffix = t.ID[len(t.ID)-6:]
	}
	label := windowLabel(t.Slug)
	if label == "" {
		label = windowLabel(t.TaskPrompt)
	}
	if label == "" {
		return "t-" + suffix
	}
	return label + "-" + suffix
}

// windowLabel slugifies s into a tmux-safe token: lowercase ASCII
// alphanumerics, with every other run collapsed to a single hyphen.
// This deliberately strips ":" and "." (tmux target separators) and
// whitespace (which would break the status-bar tab). The result is
// capped at windowLabelMaxLen on a hyphen boundary so a long prompt
// doesn't produce a giant tab. Returns "" when s carries no usable
// characters.
func windowLabel(s string) string {
	var b strings.Builder
	prevHyphen := false
	for _, r := range strings.ToLower(s) {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		switch {
		case isAlnum:
			if b.Len() >= windowLabelMaxLen {
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
	SchemaVersion int     `json:"schema_version"`
	Tracks        []Track `json:"tracks"`
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
}

// FileStore is a Store backed by <state_dir>/state.json.
//
// All access is serialized by an RWMutex. Mutations are written to a
// temp file and renamed into place so a partial write can never
// corrupt the canonical file.
type FileStore struct {
	path string

	mu     sync.RWMutex
	tracks map[string]Track
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
		path:   filepath.Join(stateDir, "state.json"),
		tracks: make(map[string]Track),
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
		migrateTrack(&t)
		fs.tracks[t.ID] = t
	}
	return nil
}

// migrateTrack upgrades a track loaded from an older schema in place.
// v1 had no Kind; infer it from the branch (pr/* came from review
// tracks) and default everything else to work. v2 spelled the
// in-review status "pr"; it's "pr open" from v3 on. (The v2 flat pr_*
// fields are folded into PRs by UnmarshalJSON, which has to run at
// decode time to see them at all.)
func migrateTrack(t *Track) {
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
}

// UnmarshalJSON decodes a Track, folding the pre-v3 single-PR fields
// (pr_url, pr_state, pr_draft, pr_review_state, pr_comments) into the
// PRs list. Done here rather than in migrateTrack because those fields
// no longer exist on Track — this is the only place they're still
// visible. Tracks decoded from the daemon socket get the same treatment,
// so an older state file needs no rewrite before it can be served.
func (t *Track) UnmarshalJSON(data []byte) error {
	type track Track // shed the method set to avoid recursing
	var aux struct {
		track
		LegacyPRURL         string `json:"pr_url"`
		LegacyPRState       string `json:"pr_state"`
		LegacyPRDraft       bool   `json:"pr_draft"`
		LegacyPRReviewState string `json:"pr_review_state"`
		LegacyPRComments    int    `json:"pr_comments"`
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
	return nil
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
	mu     sync.RWMutex
	tracks map[string]Track
}

// NewMemoryStore returns an empty in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tracks: make(map[string]Track)}
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

// Compile-time interface checks.
var (
	_ Store = (*FileStore)(nil)
	_ Store = (*MemoryStore)(nil)
)
