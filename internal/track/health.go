package track

// Mergeable is whether a PR merges into its base cleanly, as GitHub
// computed it.
type Mergeable string

const (
	MergeUnknown   Mergeable = "" // GitHub hasn't computed it yet
	MergeClean     Mergeable = "clean"
	MergeConflicts Mergeable = "conflicts"
)

// Checks is where a PR's CI checks are, on its head commit.
type Checks string

const (
	ChecksNone    Checks = "" // none ran, or none were read
	ChecksPending Checks = "pending"
	ChecksPassing Checks = "passing"
	ChecksFailing Checks = "failing"
)

// Health is what a PR's agent may have to act on: conflicts with its
// base, and failing checks.
type Health struct {
	Base      string // the branch it merges into
	Head      string // the commit the checks ran on
	Mergeable Mergeable
	Checks    Checks
	// Failing names the checks that failed, when Checks is failing.
	Failing []string
}
