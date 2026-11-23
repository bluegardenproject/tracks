package tracks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/train"
)

// trainsFixture is a fixture with track r, its window and a reviewer
// on engine, not yet started.
func trainsFixture(t *testing.T, engine string) (*fixture, *fakeReviewers, string) {
	t.Helper()
	f := newFixture(t)
	worktree := t.TempDir()
	if err := f.store.AddTrack(context.Background(), track.Track{ID: "r", Kind: track.Work, Name: "r", Engine: "claude",
		Repos: []track.Repo{{Name: "api", Worktree: worktree}}}); err != nil {
		t.Fatal(err)
	}
	f.windows.windows = append(f.windows.windows, trackwin.Info{Window: "@r", Track: "r", Name: "r"})
	panes := &fakeReviewers{}
	f.svc.Reviewers = panes
	f.svc.Settings = func() (settings.Settings, error) {
		return settings.Settings{Engines: f.engines, Agents: settings.Agents{Reviewer: &settings.TrackType{Engine: engine, Model: "m1"}}}, nil
	}
	return f, panes, worktree
}

func startReview(t *testing.T, f *fixture, worktree string) string {
	t.Helper()
	if _, err := f.svc.StartReview(context.Background(), ReviewRequest{ID: "r", Dir: worktree,
		Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "+changed\n"}}}}); err != nil {
		t.Fatal(err)
	}
	run, err := os.ReadFile(filepath.Join(f.svc.reviewDir("r"), "run"))
	if err != nil {
		t.Fatal(err)
	}
	return string(run)
}

func oneTrain(t *testing.T, f *fixture) train.Train {
	t.Helper()
	trains, err := f.svc.Trains(context.Background(), "r")
	if err != nil || len(trains) != 1 {
		t.Fatalf("trains %+v, %v; want the reviewer", trains, err)
	}
	return trains[0]
}

func TestTrainsListTheReviewer(t *testing.T) {
	ctx := context.Background()
	f, panes, worktree := trainsFixture(t, "cursor")
	if trains, err := f.svc.Trains(ctx, "r"); err != nil || len(trains) != 0 {
		t.Fatalf("trains %+v, %v before a review; want none", trains, err)
	}
	run := startReview(t, f, worktree)
	tr := oneTrain(t, f)
	if tr.Handle != "reviewer" || tr.Agent != train.Reviewer || tr.Engine != "Cursor (m1)" || tr.Status != track.Active || tr.Outcome.ID != "" {
		t.Errorf("reviewing: %+v", tr)
	}
	if err := f.svc.ReviewReply(ctx, "r", run, "Fine.\nREVIEW OUTCOME: pass"); err != nil {
		t.Fatal(err)
	}
	if tr := oneTrain(t, f); tr.Status != track.Done || tr.Outcome != train.Pass {
		t.Errorf("report in: %s %s, want done pass", tr.Status.ID, tr.Outcome.ID)
	}
	question := filepath.Join(f.svc.reviewDir("r"), "question.md")
	if err := os.WriteFile(question, []byte("Why?"), 0o600); err != nil {
		t.Fatal(err)
	}
	if tr := oneTrain(t, f); tr.Status != track.Active || tr.Outcome != train.Pass {
		t.Errorf("asked a question: %s %s, want active pass", tr.Status.ID, tr.Outcome.ID)
	}
	if err := os.Remove(question); err != nil {
		t.Fatal(err)
	}
	panes.state = ReviewerState{}
	if trains, err := f.svc.Trains(ctx, "r"); err != nil || len(trains) != 0 {
		t.Errorf("pane gone: trains %+v, %v; want none", trains, err)
	}
	if _, err := f.svc.Trains(ctx, "gone"); err == nil {
		t.Error("a track that's gone listed trains")
	}
}

func TestTrainsSeeClaudeAskTrust(t *testing.T) {
	f, panes, worktree := trainsFixture(t, "claude")
	startReview(t, f, worktree)
	panes.screen = " Quick safety check: Is this a project you created or one you trust?\n ❯ No, exit\n   Yes, I trust this folder"
	if tr := oneTrain(t, f); tr.Status != track.ActionRequired || tr.Engine != "Claude Code (m1)" {
		t.Errorf("asking trust: %s on %q, want action required on Claude Code (m1)", tr.Status.ID, tr.Engine)
	}
}

func TestEndTrain(t *testing.T) {
	ctx := context.Background()
	f, panes, worktree := trainsFixture(t, "cursor")
	var problem Problem
	if _, err := f.svc.EndTrain(ctx, "r", "reviewer"); !errors.As(err, &problem) {
		t.Errorf("ending a reviewer that isn't there: %v, want a Problem", err)
	}
	startReview(t, f, worktree)
	for _, handle := range []string{"detective", "Reviewer"} {
		if _, err := f.svc.EndTrain(ctx, "r", handle); !errors.As(err, &problem) {
			t.Errorf("ending %q: %v, want a Problem", handle, err)
		}
	}
	name, err := f.svc.EndTrain(ctx, "r", "reviewer")
	if err != nil || name != "r" || len(panes.closed) != 2 {
		t.Fatalf("ending the reviewer: %q, %v, closed %v", name, err, panes.closed)
	}
	if trains, err := f.svc.Trains(ctx, "r"); err != nil || len(trains) != 0 {
		t.Errorf("trains %+v, %v after the reviewer ended; want none", trains, err)
	}
}

// A review's run goes with its track's window, so tracks review --wait
// doesn't wait on a reviewer that's gone.
func TestReviewGoesWithTheTrack(t *testing.T) {
	ctx := context.Background()
	for _, end := range []struct {
		name string
		do   func(f *fixture) error
	}{
		{"End", func(f *fixture) error { return f.svc.End(ctx, "r") }},
		{"Sweep", func(f *fixture) error { f.windows.windows = nil; return f.svc.Sweep(ctx) }},
		{"Interrupt", func(f *fixture) error { f.windows.windows = nil; _, err := f.svc.Interrupt(ctx); return err }},
	} {
		f, _, worktree := trainsFixture(t, "cursor")
		startReview(t, f, worktree)
		if err := end.do(f); err != nil {
			t.Fatalf("%s: %v", end.name, err)
		}
		if exists(filepath.Join(f.svc.reviewDir("r"), "run")) {
			t.Errorf("after %s the review's run is still there", end.name)
		}
	}
}
