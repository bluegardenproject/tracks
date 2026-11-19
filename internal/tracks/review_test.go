package tracks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

type fakeReviewers struct {
	opened []trackwin.Process
	dirs   []string
	closed []string
}

func (f *fakeReviewers) OpenReviewer(_, dir string, p trackwin.Process) error {
	f.opened, f.dirs = append(f.opened, p), append(f.dirs, dir)
	return nil
}

func (f *fakeReviewers) CloseReviewer(track, window string) error {
	f.closed = append(f.closed, track+" "+window)
	return nil
}

func TestReview(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	worktree := t.TempDir()
	if err := f.store.AddTrack(ctx, track.Track{ID: "r", Kind: track.Work, Name: "r", Engine: "claude", Candor: 7,
		Repos: []track.Repo{{Name: "api", Worktree: worktree}}}); err != nil {
		t.Fatal(err)
	}
	f.windows.windows = append(f.windows.windows, trackwin.Info{Window: "@r", Track: "r", Name: "r"})
	panes := &fakeReviewers{}
	f.svc.Reviewers = panes
	f.svc.Settings = func() (settings.Settings, error) {
		return settings.Settings{Engines: f.engines, Agents: settings.Agents{Reviewer: &settings.TrackType{Engine: "cursor", Model: "gpt-5"}}}, nil
	}

	sub := filepath.Join(worktree, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	name, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: sub, Diff: "+changed\n"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "Cursor (gpt-5)" || len(panes.opened) != 1 || panes.dirs[0] != worktree || len(panes.closed) != 1 || panes.closed[0] != "r @r" {
		t.Fatalf("name %q, opened %+v in %v, closed %v; want Cursor's reviewer in the worktree, after the one there", name, panes.opened, panes.dirs, panes.closed)
	}
	cmd := panes.opened[0].Command
	for _, want := range []string{"TRACKS_ID='r'", "TRACKS_REVIEW=1", "sh -c", "agent", "--mode", "ask", "gpt-5", "brief.md", "--reply-file"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("the reviewer's command lacks %s: %s", want, cmd)
		}
	}
	dir := f.svc.reviewDir("r")
	brief, _ := os.ReadFile(filepath.Join(dir, "brief.md"))
	if !strings.Contains(string(brief), "Candor level: 7/10") || !strings.Contains(string(brief), "+changed") {
		t.Errorf("brief:\n%s", brief)
	}
	run, _ := os.ReadFile(filepath.Join(dir, "run"))
	if !strings.Contains(cmd, "--reviewer "+string(run)) {
		t.Errorf("the reviewer's reply doesn't name its run %s: %s", run, cmd)
	}

	done := make(chan string, 1)
	go func() {
		report, err := f.svc.WaitReview(ctx, "r")
		if err != nil {
			report = err.Error()
		}
		done <- report
	}()
	for _, reply := range []struct{ run, text string }{
		{"old", "Stale.\nREVIEW OUTCOME: blocked"},
		{string(run), "Looking at it…"},
		{string(run), "Report.\nREVIEW OUTCOME: pass"},
		{string(run), "An answer, later.\nREVIEW OUTCOME: blocked"},
	} {
		if err := f.svc.ReviewReply(ctx, "r", reply.run, reply.text); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case report := <-done:
		if report != "Report.\nREVIEW OUTCOME: pass" {
			t.Errorf("report %q; want the run's first reply with an outcome", report)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("WaitReview didn't return the report")
	}

	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: sub, Diff: "+again\n"}); err != nil {
		t.Fatal(err)
	}
	if len(panes.closed) != 2 {
		t.Error("a new review didn't end the reviewer that was there")
	}
	defer func(d time.Duration) { reviewWait = d }(reviewWait)
	reviewWait = time.Second
	if _, err := f.svc.WaitReview(ctx, "r"); err == nil || !strings.Contains(err.Error(), "still runs in its pane") {
		t.Errorf("a new review returned the old report, or waited on: %v", err)
	}
	if _, err := f.svc.WaitReview(ctx, "../x"); err == nil {
		t.Error("a track that isn't there was waited for")
	}

	f.svc.Settings = func() (settings.Settings, error) {
		return settings.Settings{Engines: f.engines}, nil
	}
	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: worktree, Diff: "+claude\n"}); err != nil {
		t.Fatal(err)
	}
	run, _ = os.ReadFile(filepath.Join(dir, "run"))
	settingsFile, _ := os.ReadFile(filepath.Join(dir, "hooks", "claude-settings.json"))
	if cmd := panes.opened[len(panes.opened)-1].Command; !strings.Contains(cmd, "--permission-mode") || !strings.Contains(cmd, "plan") ||
		!strings.Contains(string(settingsFile), "--reviewer "+string(run)) {
		t.Errorf("Claude's reviewer, the track's own engine: %s\nhooks:\n%s", cmd, settingsFile)
	}

	for _, other := range []string{"relative", t.TempDir(), worktree + "2", filepath.Join(worktree, "..")} {
		if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: other, Diff: "x"}); err == nil {
			t.Errorf("%s, not the track's worktree, was reviewed", other)
		}
	}
	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "gone", Dir: worktree, Diff: "x"}); err == nil {
		t.Error("a track that isn't there was reviewed")
	}
}
