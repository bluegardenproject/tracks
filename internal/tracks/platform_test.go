package tracks

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSyncTrainsOpensAndClosesThePlatform(t *testing.T) {
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
	if len(f.windows.platforms) != 0 {
		t.Fatalf("a track without trains got a Platform: %v", f.windows.platforms)
	}
	if heardChange() {
		t.Error("a track without trains was told as a change")
	}

	run := startReview(t, f, worktree)
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	cmd := f.windows.platforms["@r"]
	if !strings.Contains(cmd, "TRACKS_ID='r'") || !strings.HasSuffix(cmd, " platform") {
		t.Errorf("the Platform runs %q, want tracks platform for track r", cmd)
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
	panes.state = ReviewerState{}
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.windows.platforms) != 1 || heardChange() {
		t.Errorf("a busy track's Platform was touched (%v) or a change told", f.windows.platforms)
	}
	release()
	panes.state = ReviewerState{Pane: "%9"}

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
	if len(f.windows.platforms) != 0 {
		t.Errorf("the Platform stayed after the last train went: %v", f.windows.platforms)
	}
	if !heardChange() {
		t.Error("the last train going wasn't told as a change")
	}
}
