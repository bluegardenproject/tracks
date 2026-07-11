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
// v2 adds Track.Kind. v1 tracks are migrated on load (see load()).
const CurrentSchemaVersion = 2

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
)

// Worktreeless reports whether tracks of this kind run without their
// own worktree (pointing at the primary checkout read-only). Such
// tracks skip worktree creation, branch tracking, diff aggregation,
// and worktree removal.
func (k Kind) Worktreeless() bool {
	return k == KindAsk || k == KindPlan
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

	// StatusDone means the Claude process exited cleanly.
	StatusDone Status = "done"

	// StatusPR means Claude exited after opening a pull request, but the
	// track is deliberately kept open: review comments, discussion, and
	// follow-up commits are still likely. It is *non-terminal* (see
	// IsTerminal) so the worktree is preserved and token usage keeps
	// accruing. The PR watcher drives it to Done once the PR is
	// merged/closed; an explicit End/Kill also finalizes it.
	StatusPR Status = "pr"

	// StatusErrored means the Claude process exited non-zero, or
	// `tracks` was unable to spawn it / set up the worktrees.
	StatusErrored Status = "errored"

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

// Live reports whether a service in this status still has a running
// process group that teardown must signal. Every pre-terminal status
// (starting, running, ready) counts; failed/stopped do not.
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

	// Kind is the track type (work/review/ask/plan). Empty in v1 files;
	// migrated to KindWork on load. Drives worktree handling and how
	// Claude is launched.
	Kind Kind `json:"kind,omitempty"`

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

	// PRURL is set when the daemon sees a TRACKS_PR_URL=<url> marker
	// in the log. Empty otherwise.
	PRURL string `json:"pr_url,omitempty"`

	// PRState / PRDraft / PRReviewState / PRComments are filled by
	// the supervisor's gh-poll goroutine once a PRURL is known.
	// Empty until that first poll lands.
	PRState       string `json:"pr_state,omitempty"` // OPEN / CLOSED / MERGED
	PRDraft       bool   `json:"pr_draft,omitempty"`
	PRReviewState string `json:"pr_review_state,omitempty"` // APPROVED / CHANGES_REQUESTED / REVIEW_REQUIRED
	PRComments    int    `json:"pr_comments,omitempty"`

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
	// holds the reason the last creation attempt failed.
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
	Repos      []string `json:"repos,omitempty"`
	TaskPrompt string   `json:"task_prompt,omitempty"`
	Slug       string   `json:"slug,omitempty"`
	ReviewRef  string   `json:"review_ref,omitempty"`
	Kind       string   `json:"kind,omitempty"`
}

// IsTerminal reports whether s is one of the end-state statuses.
func (s Status) IsTerminal() bool {
	return s == StatusDone || s == StatusErrored
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
// tracks) and default everything else to work.
func migrateTrack(t *Track) {
	if t.Kind == "" {
		if strings.HasPrefix(t.Branch, "pr/") {
			t.Kind = KindReview
		} else {
			t.Kind = KindWork
		}
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
