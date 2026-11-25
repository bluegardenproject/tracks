package tracks

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/train"
)

// twoTargets is a fixture with track r on two repos, its two worktrees
// targets of reviews of their own.
func twoTargets(t *testing.T, engine string) (*fixture, *fakeReviewers, string, string) {
	t.Helper()
	f := newFixture(t)
	api, web := t.TempDir(), t.TempDir()
	if err := f.store.AddTrack(context.Background(), track.Track{ID: "r", Kind: track.Work, Name: "r", Engine: "claude",
		Repos: []track.Repo{{Name: "api", Worktree: api}, {Name: "web", Worktree: web}}}); err != nil {
		t.Fatal(err)
	}
	f.windows.windows = append(f.windows.windows, trackwin.Info{Window: "@r", Track: "r", Name: "r"})
	panes := &fakeReviewers{}
	f.svc.Reviewers = panes
	f.svc.Settings = func() (settings.Settings, error) {
		return settings.Settings{Engines: f.engines, Agents: settings.Agents{Reviewer: &settings.Agent{Engine: engine, Model: "m1"}}}, nil
	}
	return f, panes, api, web
}

func reviewOn(t *testing.T, f *fixture, dir string) Started {
	t.Helper()
	s, err := f.svc.StartReview(context.Background(), ReviewRequest{ID: "r", Dir: dir,
		Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "+changed\n"}}}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func runOfHandle(t *testing.T, f *fixture, handle string) string {
	t.Helper()
	return readTrim(filepath.Join(f.svc.trainDir("r", handle), "run"))
}

// A review of another target starts beside the running reviewers; one
// of the same target replaces that target's only.
func TestReviewersByTarget(t *testing.T) {
	ctx := context.Background()
	f, panes, api, web := twoTargets(t, "cursor")
	first := reviewOn(t, f, api)
	second := reviewOn(t, f, web)
	if first.Handle == second.Handle || first.Target != api || second.Target != web || len(panes.closed) != 0 {
		t.Fatalf("started %+v and %+v, closed %v; want two reviewers, one per worktree", first, second, panes.closed)
	}
	trains, err := f.svc.Trains(ctx, "r")
	if err != nil || len(trains) != 2 {
		t.Fatalf("trains %+v, %v; want both reviewers", trains, err)
	}
	for _, tr := range trains {
		want := map[string]string{first.Handle: api, second.Handle: web}[tr.Handle]
		if tr.Target != want {
			t.Errorf("%s reviews %q, want %q", tr.Handle, tr.Target, want)
		}
	}

	third := reviewOn(t, f, api)
	if len(panes.closed) != 1 || panes.closed[0] != "r "+first.Handle || third.Handle == first.Handle {
		t.Fatalf("closed %v, new %s; want only %s replaced, by a new handle", panes.closed, third.Handle, first.Handle)
	}
	if exists(f.svc.trainDir("r", first.Handle)) || !exists(f.svc.trainDir("r", second.Handle)) {
		t.Error("the replaced reviewer's folder is left, or the other's went")
	}
	msgs := relayOf(t, f)
	if last := msgs[len(msgs)-2]; last.Body != "@"+first.Handle+" left: a new review replaces it." {
		t.Errorf("notice %q; want the replaced reviewer leaving", last.Body)
	}

	// Each reply goes to the reviewer whose run it names.
	if err := f.svc.ReviewReply(ctx, "r", runOfHandle(t, f, second.Handle), "Web.\nREVIEW OUTCOME: blocked"); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ReviewReply(ctx, "r", runOfHandle(t, f, third.Handle), "API.\nREVIEW OUTCOME: pass"); err != nil {
		t.Fatal(err)
	}
	for h, want := range map[string]string{second.Handle: "Web.", third.Handle: "API."} {
		if b, _ := os.ReadFile(filepath.Join(f.svc.trainDir("r", h), "report.md")); !strings.HasPrefix(string(b), want) {
			t.Errorf("%s's report %q, want %q", h, b, want)
		}
	}
	reports := 0
	for _, m := range relayOf(t, f) {
		if m.Kind == relay.Report {
			reports++
			if m.From != second.Handle && m.From != third.Handle {
				t.Errorf("a report from %s", m.From)
			}
		}
	}
	if reports != 2 {
		t.Errorf("%d reports on the relay, want one per reviewer", reports)
	}

	// The Platform's tick badges each reviewer's own session.
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	for h, want := range map[string]string{second.Handle: actions.Badges(track.Done, train.Blocked), third.Handle: actions.Badges(track.Done, train.Pass)} {
		if got := panes.badges[trackwin.TrainSession("r", h)]; got != want {
			t.Errorf("%s's badge %q, want %q", h, got, want)
		}
	}
	who, err := f.svc.RelayWho(ctx, "r")
	if err != nil || len(who) != 3 {
		t.Errorf("relay who %+v, %v; want the track and both reviewers", who, err)
	}

	if _, err := f.svc.EndTrain(ctx, "r", "reviewer"); err == nil || !strings.Contains(err.Error(), "could be") {
		t.Errorf("ending by a prefix both have: %v, want it to ask which", err)
	}
	if _, err := f.svc.EndTrain(ctx, "r", second.Handle); err != nil {
		t.Fatal(err)
	}
	if tr := oneTrain(t, f); tr.Handle != third.Handle {
		t.Errorf("left %s, want %s", tr.Handle, third.Handle)
	}
	if err := f.svc.dropReview(ctx, "r", "its track ended"); err != nil {
		t.Fatal(err)
	}
	if states, _ := f.svc.trainStates("r", ""); len(states) != 0 {
		t.Errorf("trains left after the track ended: %+v", states)
	}
}

// --wait and --ask need --to only when more than one reviewer runs.
func TestReviewAddressing(t *testing.T) {
	ctx := context.Background()
	f, panes, api, web := twoTargets(t, "claude")
	var problem Problem
	if _, err := f.svc.WaitReview(ctx, "r", ""); !errors.As(err, &problem) || !strings.Contains(err.Error(), "no review running") {
		t.Errorf("waiting with no reviewer: %v", err)
	}
	a := reviewOn(t, f, api)
	if err := f.svc.ReviewReply(ctx, "r", runOfHandle(t, f, a.Handle), "A.\nREVIEW OUTCOME: pass"); err != nil {
		t.Fatal(err)
	}
	// A folder whose pane was closed some other way is no reviewer.
	orphan := f.svc.trainDir("r", "reviewer-999999")
	if err := os.MkdirAll(orphan, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"agent": "reviewer", "run": "gone"} {
		if err := os.WriteFile(filepath.Join(orphan, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if report, err := f.svc.WaitReview(ctx, "r", ""); err != nil || report != "A.\nREVIEW OUTCOME: pass" {
		t.Errorf("one reviewer, no --to: %q, %v", report, err)
	}
	b := reviewOn(t, f, web)
	if err := f.svc.ReviewReply(ctx, "r", runOfHandle(t, f, b.Handle), "B.\nREVIEW OUTCOME: blocked"); err != nil {
		t.Fatal(err)
	}
	_, err := f.svc.WaitReview(ctx, "r", "")
	if !errors.As(err, &problem) || !strings.Contains(err.Error(), "--to") || !strings.Contains(err.Error(), "@"+a.Handle+" on "+api) ||
		!strings.Contains(err.Error(), "@"+b.Handle+" on "+web) {
		t.Errorf("two reviewers, no --to: %v; want them listed with their targets", err)
	}
	if _, err := f.svc.AskReview(ctx, "r", "", "Why?"); !errors.As(err, &problem) || !strings.Contains(err.Error(), "--to") {
		t.Errorf("asking two reviewers, no --to: %v", err)
	}
	if _, err := f.svc.WaitReview(ctx, "r", "reviewer"); err == nil || !strings.Contains(err.Error(), "could be") {
		t.Errorf("a prefix both have: %v", err)
	}
	if _, err := f.svc.WaitReview(ctx, "r", "nobody"); !errors.As(err, &problem) {
		t.Errorf("a handle no reviewer has: %v", err)
	}
	for h, want := range map[string]string{"@" + a.Handle: "A.", b.Handle: "B."} {
		if report, err := f.svc.WaitReview(ctx, "r", h); err != nil || !strings.HasPrefix(report, want) {
			t.Errorf("--to %s: %q, %v; want %q", h, report, err, want)
		}
	}

	if _, err := f.svc.AskReview(ctx, "r", b.Handle, "Is B real?"); err != nil {
		t.Fatal(err)
	}
	pane := panes.state.Pane
	if len(panes.typed) != 1 || panes.typed[0] != pane+" "+questionLead+"Is B real?" {
		t.Errorf("typed %q; want the question in %s's pane %s", panes.typed, b.Handle, pane)
	}
	if !exists(filepath.Join(f.svc.trainDir("r", b.Handle), "question.md")) || exists(filepath.Join(f.svc.trainDir("r", a.Handle), "question.md")) {
		t.Error("the question went to the other reviewer's folder")
	}
	// A asked nothing: its reply now is talk, B's is the answer.
	if err := f.svc.ReviewReply(ctx, "r", runOfHandle(t, f, a.Handle), "Talk."); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.ReviewReply(ctx, "r", runOfHandle(t, f, b.Handle), "Yes."); err != nil {
		t.Fatal(err)
	}
	if answer, err := f.svc.WaitReview(ctx, "r", b.Handle); err != nil || answer != "Yes." {
		t.Errorf("B's answer %q, %v", answer, err)
	}
	if report, err := f.svc.WaitReview(ctx, "r", a.Handle); err != nil || !strings.HasPrefix(report, "A.") {
		t.Errorf("A, asked nothing, still gives its report: %q, %v", report, err)
	}
}

// A review's folder from before trains had folders of their own is read
// where it is, as a running Cursor reviewer writes its report there,
// and its hook's replies still reach it.
func TestReviewFolderMigrates(t *testing.T) {
	ctx := context.Background()
	f, panes, worktree := trainsFixture(t, "claude")
	old := f.svc.legacyReviewDir("r")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	sess, _ := json.Marshal(reviewSession{Engine: "claude", Program: "claude", Workspace: worktree})
	for name, text := range map[string]string{"run": "r1", "handle": "reviewer-111111", "session.json": string(sess)} {
		if err := os.WriteFile(filepath.Join(old, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	panes.state = ReviewerState{Handle: "reviewer-111111", Pane: "%3", Session: trackwin.TrainSession("r", "reviewer-111111")}
	tr := oneTrain(t, f)
	if tr.Handle != "reviewer-111111" || tr.Target != worktree || !exists(old) {
		t.Fatalf("train %+v, the old folder there: %v; want it read in place, its target its workspace", tr, exists(old))
	}
	if err := f.svc.ReviewReply(ctx, "r", "r1", "Old.\nREVIEW OUTCOME: pass"); err != nil {
		t.Fatal(err)
	}
	if report, err := f.svc.WaitReview(ctx, "r", ""); err != nil || report != "Old.\nREVIEW OUTCOME: pass" {
		t.Errorf("the moved review's report %q, %v", report, err)
	}
	next := reviewOn(t, f, worktree)
	if len(panes.closed) != 1 || panes.closed[0] != "r reviewer-111111" || next.Handle == "reviewer-111111" || exists(old) {
		t.Errorf("closed %v; a new review of its target replaces the old one, its folder gone", panes.closed)
	}
}

// A reviewer that didn't start leaves no folder that would pass for a
// reviewer to --wait and --ask.
func TestFailedStartLeavesNoReviewer(t *testing.T) {
	f, panes, worktree := trainsFixture(t, "claude")
	panes.openErr = errors.New("tmux is gone")
	if _, err := f.svc.StartReview(context.Background(), ReviewRequest{ID: "r", Dir: worktree,
		Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "+x\n"}}}}); err == nil {
		t.Fatal("a reviewer whose pane didn't open started")
	}
	if states, _ := f.svc.trainStates("r", ""); len(states) != 0 {
		t.Errorf("folders left: %+v", states)
	}
}

// Two reviewers asking for trust are each told once.
func TestReviewersAskTrustEachTold(t *testing.T) {
	ctx := context.Background()
	f, panes, api, web := twoTargets(t, "claude")
	var told []Notice
	f.svc.Notify = func(n Notice) { told = append(told, n) }
	a, b := reviewOn(t, f, api), reviewOn(t, f, web)
	panes.screen = " Quick safety check: Is this a project you created or one you trust?\n ❯ No, exit\n   Yes, I trust this folder"
	for range 3 {
		if err := f.svc.SyncTrains(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(told) != 2 || !strings.Contains(told[0].Title+told[1].Title, "@"+a.Handle) || !strings.Contains(told[0].Title+told[1].Title, "@"+b.Handle) {
		t.Errorf("told %+v; want each reviewer once", told)
	}
}
