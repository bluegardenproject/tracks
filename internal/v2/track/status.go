package track

import "time"

// Status is one value of a track's status. ID is how it's stored and
// sent, Label how it's shown, Badge the theme state its badge is drawn
// in: BadgeInfo, BadgeWarning or BadgeDanger. A lower Priority is shown first where
// only one fits; Attention says the track needs the user.
type Status struct {
	ID        string
	Label     string
	Badge     string
	Priority  int
	Attention bool
}

// The badges' theme states: a badge is drawn in state.<badge>.bg and
// state.<badge>.text.
const (
	BadgeInfo    = "info"
	BadgeWarning = "warning"
	BadgeDanger  = "danger"
)

// The track statuses. Adding one is an entry here and in Statuses, the
// event that sets it in Apply, and their tests.
var (
	Error          = Status{ID: "error", Label: "error", Badge: BadgeDanger, Priority: -1, Attention: true}
	ActionRequired = Status{ID: "action_required", Label: "action required", Badge: BadgeWarning, Attention: true}
	Exited         = Status{ID: "exited", Label: "agent exited", Badge: BadgeWarning, Priority: 1}
	Active         = Status{ID: "active", Label: "active", Badge: BadgeInfo, Priority: 2}
	Done           = Status{ID: "done", Label: "done", Badge: BadgeInfo, Priority: 3}
	Closed         = Status{ID: "closed", Label: "closed", Badge: BadgeInfo, Priority: 4}
)

// Statuses are every track status, by priority.
var Statuses = []Status{Error, ActionRequired, Exited, Active, Done, Closed}

// How the agent exited, in State.Exit: with code 0, or any other.
const (
	ExitOK     = "exited"
	ExitFailed = "failed"
)

// State is what a track's status is derived from. Only Apply changes
// it.
type State struct {
	ClosedAt   time.Time // zero while its window is open
	CleanedAt  time.Time // set when Archive removed its worktrees
	ArchivedAt time.Time // zero while it's listed in Station
	Waiting    bool      // its agent waits on a dialog in its window
	Exit       string    // how its agent exited: "" while it runs, ExitOK or ExitFailed
}

// Open reports whether the track's window is still open.
func (s State) Open() bool { return s.ClosedAt.IsZero() }

// Cleaned reports whether Archive removed the track's worktrees and
// branches. An unarchived track stays cleaned until it's resumed.
func (s State) Cleaned() bool { return !s.CleanedAt.IsZero() }

// Archived reports whether the track was taken out of Station.
func (s State) Archived() bool { return !s.ArchivedAt.IsZero() }

// Status is the track status s gives.
func (s State) Status() Status {
	switch {
	case s.Archived():
		return Closed
	case !s.Open():
		return Done
	case s.Exit == ExitFailed:
		return Error
	case s.Exit == ExitOK:
		return Exited
	case s.Waiting:
		return ActionRequired
	}
	return Active
}

// Event is something that happened to a track, which may change its
// status.
type Event string

const (
	Created Event = "created"
	Resumed Event = "resumed"
	Ended   Event = "ended"
	// Cleaned says Archive removed the worktrees and branches.
	Cleaned Event = "cleaned"
	// Archived takes an ended track out of Station, closing it;
	// Unarchived puts it back, done.
	Archived   Event = "archived"
	Unarchived Event = "unarchived"
	// AgentWaiting and AgentWorking say the agent opened a dialog in the
	// track's window, and that it's gone.
	AgentWaiting Event = "agent.waiting"
	AgentWorking Event = "agent.working"
	// AgentExited and AgentFailed say the agent exited, with code 0 or
	// with another, leaving a shell in its pane.
	AgentExited Event = "agent.exited"
	AgentFailed Event = "agent.failed"
)

// Valid reports whether e is an event Apply knows.
func (e Event) Valid() bool {
	switch e {
	case Created, Resumed, Ended, Cleaned, Archived, Unarchived, AgentWaiting, AgentWorking, AgentExited, AgentFailed:
		return true
	}
	return false
}

// Apply is s after e, which happened at at. An event that doesn't fit
// the state, such as the agent's on an ended track or on one whose
// agent exited, changes nothing.
func (s State) Apply(e Event, at time.Time) State {
	switch e {
	case Created, Resumed:
		return State{}
	case Ended:
		if s.Open() {
			return State{ClosedAt: at}
		}
	case Cleaned:
		if !s.Open() && !s.Cleaned() {
			s.CleanedAt = at
		}
	case Archived:
		if !s.Open() && !s.Archived() {
			s.ArchivedAt = at
		}
	case Unarchived:
		s.ArchivedAt = time.Time{}
	case AgentWaiting, AgentWorking:
		if s.Open() && s.Exit == "" {
			s.Waiting = e == AgentWaiting
		}
	case AgentExited, AgentFailed:
		if s.Open() {
			s.Waiting, s.Exit = false, ExitOK
			if e == AgentFailed {
				s.Exit = ExitFailed
			}
		}
	}
	return s
}
