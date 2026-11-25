package tracks

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/relay"
)

func TestSyncTrainsKeepsThePlatformOpen(t *testing.T) {
	ctx := context.Background()
	f, panes, worktree := trainsFixture(t, "cursor")
	f.svc.Changes = &Changes{}
	changed, cancel := f.svc.Changes.Subscribe()
	defer cancel()
	<-changed
	heardChange := func() bool {
		select {
		case <-changed:
			return true
		case <-time.After(50 * time.Millisecond):
			return false
		}
	}

	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	cmd := f.windows.platforms["@r"]
	if !strings.Contains(cmd, "TRACKS_ID='r'") || !strings.HasSuffix(cmd, " platform") {
		t.Errorf("a track without trains: the Platform runs %q, want tracks platform for track r", cmd)
	}
	if heardChange() {
		t.Error("a track without trains was told as a change")
	}

	// A Platform that closed, or a window opened before the daemon,
	// gets one on the next tick.
	delete(f.windows.platforms, "@r")
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.windows.platforms) != 1 {
		t.Fatalf("a window without a Platform didn't get one: %v", f.windows.platforms)
	}

	run := startReview(t, f, worktree)
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if !heardChange() {
		t.Error("a train that came wasn't told as a change")
	}
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if heardChange() {
		t.Error("nothing changed, yet a change was told")
	}
	if err := f.svc.ReviewReply(ctx, "r", run, "Fine.\nREVIEW OUTCOME: pass"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if !heardChange() {
		t.Error("the reviewer's report changed its status, yet no change was told")
	}

	// A busy track, being restarted or promoted, is left as it is and
	// tells no change.
	_, release, err := f.svc.hold(ctx, "r")
	if err != nil {
		t.Fatal(err)
	}
	reviewer := panes.state
	panes.state = ReviewerState{}
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.windows.platforms) != 1 || heardChange() {
		t.Errorf("a busy track's Platform was touched (%v) or a change told", f.windows.platforms)
	}
	release()
	panes.state = reviewer

	// The track's agent exits; its window and its reviewer run on.
	f.windows.windows[0].Exit = "0"
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.windows.platforms) != 1 {
		t.Errorf("the Platform of a track whose agent exited closed: %v", f.windows.platforms)
	}
	panes.state = ReviewerState{}
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.windows.platforms) != 1 {
		t.Errorf("the Platform closed after the last train went: %v", f.windows.platforms)
	}
	if !heardChange() {
		t.Error("the last train going wasn't told as a change")
	}
}

func TestAPRsCommentsShowOnThePlatform(t *testing.T) {
	ctx := context.Background()
	f, _, _ := trainsFixture(t, "cursor")
	const url = "https://github.com/acme/api/pull/7"
	if err := f.svc.SeePRs(ctx, "r", []string{url}); err != nil {
		t.Fatal(err)
	}
	f.svc.mu.Lock()
	f.svc.comments = map[string]map[string]actions.Comments{"r": {url: {Open: 2}}}
	f.svc.mu.Unlock()
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.windows.platforms) != 1 {
		t.Fatalf("a track with a PR's comments and no train has no Platform: %v", f.windows.platforms)
	}
	sig, err := f.svc.Signals(ctx, "r")
	if err != nil || len(sig.PRs) != 1 || sig.PRs[0].PR != "PR #7" || sig.PRs[0].Action != "solve 2 comments" {
		t.Errorf("Signals %+v, %v", sig, err)
	}
	f.svc.mu.Lock()
	f.svc.comments = nil
	f.svc.mu.Unlock()
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.windows.platforms) != 1 {
		t.Errorf("the Platform closed with nothing to show: %v", f.windows.platforms)
	}
}

func TestHeldGitHubEventsInSignals(t *testing.T) {
	ctx := context.Background()
	f, _, _ := trainsFixture(t, "cursor")
	f.svc.setHeld("r", []relay.Message{{From: relay.GitHubHandle}})
	if sig, err := f.svc.Signals(ctx, "r"); err != nil || !sig.Held {
		t.Errorf("Signals %+v, %v; want them held, for Collect messages", sig, err)
	}
	f.svc.setHeld("r", nil)
	if sig, err := f.svc.Signals(ctx, "r"); err != nil || sig.Held {
		t.Errorf("Signals %+v, %v; want nothing held", sig, err)
	}
}
