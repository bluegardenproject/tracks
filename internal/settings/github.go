package settings

// GitHub is Settings → GitHub: what Tracks does with @github's events on
// a side track's PR (see docs/design/trains.md, "Side tracks and
// GitHub's events"). Each is its default unless set.
type GitHub struct {
	Conflicts     Policy `yaml:"conflicts,omitempty"`
	CIFailed      Policy `yaml:"ci_failed,omitempty"`
	ReviewThreads Policy `yaml:"review_threads,omitempty"`
}

// Policy is what happens on an event.
type Policy string

const (
	// PolicyAuto: Tracks tells the side track to handle it, and the
	// main track, as information.
	PolicyAuto Policy = "auto"
	// PolicyMain: the main track's agent decides, and may ask the user.
	PolicyMain Policy = "main"
	// PolicyAsk: the main track asks the user.
	PolicyAsk Policy = "ask"
)

// The events a policy is set for.
const (
	EventConflicts     = "conflicts"
	EventCIFailed      = "ci_failed"
	EventReviewThreads = "review_threads"
)

// Policy is the policy for event: its setting, or its default for none
// or one this build doesn't know.
func (g GitHub) Policy(event string) Policy {
	var set, def Policy
	switch event {
	case EventConflicts:
		set, def = g.Conflicts, PolicyAuto
	case EventCIFailed:
		set, def = g.CIFailed, PolicyAuto
	case EventReviewThreads:
		set, def = g.ReviewThreads, PolicyMain
	default:
		return PolicyMain
	}
	switch set {
	case PolicyAuto, PolicyMain, PolicyAsk:
		return set
	}
	return def
}
