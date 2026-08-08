package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/state"
)

// A track that opens a PR while Claude keeps working (the stacked-PR
// flow) ends up with two goroutines refreshing its token usage: the pane
// watcher every fifth tick, and the PR watcher on its own. Both used to
// write sup.lastUsageSig unsynchronised. Run under -race.
//
// The transcript is grown while the readers run, so the signature keeps
// changing and every call actually reaches the write — an unchanging
// transcript would return early and never exercise the race.
func TestRefreshUsageFromTwoWatchersIsRaceFree(t *testing.T) {
	const sessionID = "5f1b8a30-0000-4000-8000-000000000000"

	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	projectDir := filepath.Join(configDir, "projects", "-tmp-worktree")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript, err := os.Create(filepath.Join(projectDir, sessionID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer transcript.Close()

	srv := newReadinessTestServer(t)
	tr := state.Track{
		ID:        "trk-race",
		Status:    state.StatusRunning,
		SessionID: sessionID,
		Repos:     []state.TrackRepo{{Name: "repo", Path: t.TempDir()}},
	}
	if err := srv.store.Put(tr); err != nil {
		t.Fatal(err)
	}
	sup := &supervisor{trackID: tr.ID, done: make(chan struct{}), lastPaneChangeAt: time.Now()}

	const line = `{"type":"assistant","requestId":"%d","message":{"model":"claude-sonnet-5","usage":{"input_tokens":10,"output_tokens":5}}}` + "\n"
	stop := make(chan struct{})
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			fmt.Fprintf(transcript, line, i)
			time.Sleep(time.Millisecond)
		}
	}()

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				srv.refreshUsage(sup)
			}
		}()
	}
	wg.Wait()
	close(stop)
	writer.Wait()
}

// The pane watcher and the PR watcher also share the rest of the
// supervisor's observation bookkeeping.
func TestSupervisorObservationHelpersAreRaceFree(t *testing.T) {
	sup := &supervisor{trackID: "trk", done: make(chan struct{}), lastPaneChangeAt: time.Now()}

	var wg sync.WaitGroup
	for i := range 6 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range 50 {
				sup.observePane(string(rune('a'+i)) + string(rune('0'+j%10)))
				sup.claimWaitingNotify()
				sup.claimPRWatcher()
				sup.usageSigChanged("sig")
			}
		}(i)
	}
	wg.Wait()
}

// Exactly one caller may start the PR watcher, however many poll ticks
// re-detect the same URL.
func TestClaimPRWatcherOnlySucceedsOnce(t *testing.T) {
	sup := &supervisor{done: make(chan struct{})}

	var mu sync.Mutex
	claims := 0
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if sup.claimPRWatcher() {
				mu.Lock()
				claims++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if claims != 1 {
		t.Errorf("claimPRWatcher succeeded %d times, want exactly 1", claims)
	}
}

func TestClaimWaitingNotifyThrottles(t *testing.T) {
	sup := &supervisor{done: make(chan struct{})}

	if !sup.claimWaitingNotify() {
		t.Fatal("first notification should be allowed")
	}
	if sup.claimWaitingNotify() {
		t.Error("a second notification inside the cooldown should be refused")
	}

	// Pretend the cooldown elapsed.
	sup.mu.Lock()
	sup.lastWaitingNotifyAt = time.Now().Add(-waitingNotifyMinInterval - time.Second)
	sup.mu.Unlock()
	if !sup.claimWaitingNotify() {
		t.Error("a notification after the cooldown should be allowed")
	}
}

func TestObservePaneReportsIdleOnlyAfterThreshold(t *testing.T) {
	sup := &supervisor{done: make(chan struct{}), lastPaneChangeAt: time.Now()}

	if sup.observePane("first") {
		t.Error("a pane that just changed must not read as idle")
	}
	// Same snapshot, but the clock has moved past the threshold.
	sup.mu.Lock()
	sup.lastPaneChangeAt = time.Now().Add(-paneIdleThreshold - time.Second)
	sup.mu.Unlock()
	if !sup.observePane("first") {
		t.Error("an unchanged pane past the threshold should read as idle")
	}
	// A fresh snapshot resets the clock.
	if sup.observePane("second") {
		t.Error("a changed pane must reset the idle timer")
	}
}
