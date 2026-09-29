// Package track is what a track is: its kind, its repos and the facts
// kept about it. It imports nothing internal.
package track

import "time"

// Kind is what a track does.
type Kind string

const (
	Work   Kind = "work"
	Ask    Kind = "ask"
	Plan   Kind = "plan"
	Review Kind = "review"
	Doc    Kind = "doc"
)

// Kinds are every kind, in the form's order.
var Kinds = []Kind{Work, Ask, Plan, Review, Doc}

// Valid reports whether k is a kind.
func (k Kind) Valid() bool {
	switch k {
	case Work, Ask, Plan, Review, Doc:
		return true
	}
	return false
}

// Worktrees reports whether k's tracks get worktrees of their own. The
// others read the primary checkouts.
func (k Kind) Worktrees() bool { return k == Work || k == Review }

// ReadOnly reports whether k's agent runs in a read-only mode.
func (k Kind) ReadOnly() bool { return k == Ask || k == Plan }

// Track is a track as it's stored.
type Track struct {
	ID   string
	Kind Kind
	Name string // its window's name
	// Engine is the agent CLI's id, Model what it was started with ("" is
	// the engine's own default), Session the ID that resumes it.
	Engine, Model, Session string
	Prompt                 string // as the user wrote it
	ReviewRef              string // Review: the PR link or branch
	Document               string // Doc: the resolved path
	Candor                 int    // Review, Doc
	Opinion, ClaimCheck    bool   // Doc's optional sections
	Terminal               bool
	Repos                  []Repo
	CreatedAt              time.Time
	ClosedAt               time.Time // zero while its window is open
	CleanedAt              time.Time // zero while its worktrees exist
}

// Open reports whether t's window is still open.
func (t Track) Open() bool { return t.ClosedAt.IsZero() }

// Cleaned reports whether Clean removed t's worktrees.
func (t Track) Cleaned() bool { return !t.CleanedAt.IsZero() }

// Repo is one of a track's repos. Name, Path and Base are copies, so the
// track keeps them when the repo is renamed or removed.
type Repo struct {
	RepoID   int64 // 0 once the repo is removed
	Name     string
	Path     string // the primary checkout
	Worktree string // "" for kinds without worktrees
	Branch   string
	Base     string
}

// Dir is where the agent works in r.
func (r Repo) Dir() string {
	if r.Worktree != "" {
		return r.Worktree
	}
	return r.Path
}
