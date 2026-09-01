package daemon

import "testing"

// The branch read and the usage refresh are the two heavy jobs in the
// 2s poll loop — N git subprocesses and a transcript parse. They run on
// the same divisor, so without the offset they would land in the same
// tick, and a ticker drops ticks rather than queueing them when one
// slot overruns. This pins the stagger, which otherwise lives only in a
// comment and dies silently if either constant is edited.
func TestBranchAndUsageRefreshesNeverShareATick(t *testing.T) {
	for tick := 1; tick <= 100; tick++ {
		if shouldRefreshBranches(tick) && tick%usageRefreshEveryTicks == 0 {
			t.Fatalf("tick %d runs the branch read and the usage refresh together", tick)
		}
	}
}

func TestShouldRefreshBranchesFiresOnItsDivisor(t *testing.T) {
	var fired int
	for tick := 1; tick <= 100; tick++ {
		if shouldRefreshBranches(tick) {
			fired++
		}
	}
	if want := 100 / branchRefreshEveryTicks; fired != want {
		t.Errorf("branch read fired %d times in 100 ticks, want %d", fired, want)
	}
}
