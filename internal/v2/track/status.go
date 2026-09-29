package track

import "time"

// Status is one value of a track's status. ID is how it's stored and
// sent, Label how it's shown, Token the colour token it's drawn in, by
// name. A lower Priority is shown first where only one fits; Attention
// says the track needs the user.
type Status struct {
	ID        string
	Label     string
	Token     string
	Priority  int
	Attention bool
}

// The track statuses. Adding one is an entry here and in Statuses, the
// event that sets it in Apply, and their tests.
var (
	Active         = Status{ID: "active", Label: "active", Token: "state.success.text", Priority: 1}
	ActionRequired = Status{ID: "action_required", Label: "action required", Token: "state.warning.text", Attention: true}
	Done           = Status{ID: "done", Label: "done", Token: "text.muted", Priority: 2}
	Closed         = Status{ID: "closed", Label: "closed", Token: "text.faint", Priority: 3}
)

// Statuses are every track status, by priority.
var Statuses = []Status{ActionRequired, Active, Done, Closed}

// State is what a track's status is derived from. Only Apply changes
// it.
type State struct {
	ClosedAt   time.Time // zero while its window is open
	CleanedAt  time.Time // zero while its worktrees exist
	ArchivedAt time.Time // zero while it's listed in Station
	Waiting    bool      // its agent waits on a dialog in its window
}

// Open reports whether the track's window is still open.
func (s State) Open() bool { return s.ClosedAt.IsZero() }

// Cleaned reports whether Clean removed the track's worktrees.
func (s State) Cleaned() bool { return !s.CleanedAt.IsZero() }

// Archived reports whether the track was taken out of Station.
func (s State) Archived() bool { return !s.ArchivedAt.IsZero() }

// Status is the track status s gives.
func (s State) Status() Status {
	switch {
	case s.Cleaned():
		return Closed
	case !s.Open():
		return Done
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
	Cleaned Event = "cleaned"
	// Archived takes an ended track out of Station; Unarchived puts it
	// back.
	Archived   Event = "archived"
	Unarchived Event = "unarchived"
	// AgentWaiting and AgentWorking say the agent opened a dialog in the
	// track's window, and that it's gone.
	AgentWaiting Event = "agent.waiting"
	AgentWorking Event = "agent.working"
)

// Valid reports whether e is an event Apply knows.
func (e Event) Valid() bool {
	switch e {
	case Created, Resumed, Ended, Cleaned, Archived, Unarchived, AgentWaiting, AgentWorking:
		return true
	}
	return false
}

// Apply is s after e, which happened at at. An event that doesn't fit
// the state, such as the agent's on an ended track, changes nothing.
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
		if s.Open() {
			s.Waiting = e == AgentWaiting
		}
	}
	return s
}
