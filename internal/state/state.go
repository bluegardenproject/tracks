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
	"sync"
	"time"
)

// CurrentSchemaVersion is the SchemaVersion this binary writes. Older
// files on disk are migrated when loaded; newer files are refused so
// a forward-compatible field doesn't get silently dropped.
const CurrentSchemaVersion = 1

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

	// StatusErrored means the Claude process exited non-zero, or
	// `tracks` was unable to spawn it / set up the worktrees.
	StatusErrored Status = "errored"
)

// TrackRepo is one repository participating in a track. The Name
// matches a config.Repo.Name; Path is the absolute path of the
// worktree under <state_dir>/worktrees/<track-id>/<repo-name>.
type TrackRepo struct {
	Name string `json:"name"`
	Path string `json:"path"`
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

	// Repos lists the participating worktrees, in the order they were
	// added (initial selection first, mid-session add-repo calls
	// appended).
	Repos []TrackRepo `json:"repos"`

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

	// CreatedAt is when the track entry was written.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the last time any field on this track changed.
	UpdatedAt time.Time `json:"updated_at"`

	// ExitedAt is set once Status reaches Done or Errored.
	ExitedAt *time.Time `json:"exited_at,omitempty"`

	// ExitCode is the Claude process's exit code if available.
	ExitCode *int `json:"exit_code,omitempty"`
}

// IsTerminal reports whether s is one of the end-state statuses.
func (s Status) IsTerminal() bool {
	return s == StatusDone || s == StatusErrored
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
		fs.tracks[t.ID] = t
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
