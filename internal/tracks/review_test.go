package tracks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/actions"
	"github.com/bluegardenproject/tracks/internal/review"
	"github.com/bluegardenproject/tracks/internal/settings"
	"github.com/bluegardenproject/tracks/internal/tmux"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
	"github.com/bluegardenproject/tracks/internal/train"
)

type fakeReviewers struct {
	opened    []trackwin.Process
	specs     []trackwin.TrainSpec
	dirs      []string
	closed    []string
	state     ReviewerState
	typed     []string
	respawned []string
	screen    string
	badges    map[string]string
	badgeErr  error
	openErr   error
}

func (f *fakeReviewers) CaptureReviewer(string) (string, error) { return f.screen, nil }

func (f *fakeReviewers) TypeReviewer(pane, text string) error {
	f.typed = append(f.typed, pane+" "+text)
	return nil
}

func (f *fakeReviewers) RespawnReviewer(pane, dir, command string) error {
	f.respawned = append(f.respawned, pane+" "+dir+" "+command)
	return nil
}

func (f *fakeReviewers) OpenReviewer(s trackwin.TrainSpec) error {
	if f.openErr != nil {
		return f.openErr
	}
	f.opened, f.dirs, f.specs = append(f.opened, s.Process), append(f.dirs, s.Dir), append(f.specs, s)
	f.state = ReviewerState{Pane: "%9", Session: trackwin.TrainSession(s.Track, s.Handle)}
	return nil
}

func (f *fakeReviewers) CloseReviewer(track string) error {
	f.closed = append(f.closed, track)
	f.state = ReviewerState{}
	return nil
}

func (f *fakeReviewers) Reviewer(string) (ReviewerState, error) { return f.state, nil }

func (f *fakeReviewers) Reviewers() (map[string]ReviewerState, error) {
	if f.state.Pane == "" {
		return map[string]ReviewerState{}, nil
	}
	r := f.state
	r.Attached = false
	return map[string]ReviewerState{"r": r}, nil
}

func (f *fakeReviewers) TrainBadge(session, badges string) error {
	if f.badgeErr != nil {
		return f.badgeErr
	}
	if f.badges == nil {
		f.badges = map[string]string{}
	}
	f.badges[session] = badges
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
		return settings.Settings{Engines: f.engines, Agents: settings.Agents{Reviewer: &settings.Agent{Engine: "cursor", Model: "gpt-5"}}}, nil
	}

	sub := filepath.Join(worktree, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	name, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: sub, Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "+changed\n"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if name != "Cursor (gpt-5)" || len(panes.opened) != 1 || panes.dirs[0] != worktree || len(panes.closed) != 1 || panes.closed[0] != "r" {
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
	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: sub}); err == nil {
		t.Error("an empty packet was reviewed")
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

	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: sub, Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "+again\n"}}}}); err != nil {
		t.Fatal(err)
	}
	if len(panes.closed) != 2 {
		t.Error("a new review didn't end the reviewer that was there")
	}
	defer func(d time.Duration) { reviewWait = d }(reviewWait)
	reviewWait = time.Second
	if _, err := f.svc.WaitReview(ctx, "r"); err == nil || !strings.Contains(err.Error(), "still runs") {
		t.Errorf("a new review returned the old report, or waited on: %v", err)
	}
	if _, err := f.svc.WaitReview(ctx, "../x"); err == nil {
		t.Error("a track that isn't there was waited for")
	}

	f.svc.Settings = func() (settings.Settings, error) {
		return settings.Settings{Engines: f.engines}, nil
	}
	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: worktree, Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "+claude\n"}}}}); err != nil {
		t.Fatal(err)
	}
	run, _ = os.ReadFile(filepath.Join(dir, "run"))
	settingsFile, _ := os.ReadFile(filepath.Join(dir, "hooks", "claude-settings.json"))
	if cmd := panes.opened[len(panes.opened)-1].Command; !strings.Contains(cmd, "--permission-mode") || !strings.Contains(cmd, "plan") ||
		!strings.Contains(string(settingsFile), "--reviewer "+string(run)) {
		t.Errorf("Claude's reviewer, the track's own engine: %s\nhooks:\n%s", cmd, settingsFile)
	}

	for _, other := range []string{"relative", t.TempDir(), worktree + "2", filepath.Join(worktree, "..")} {
		if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: other, Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "x"}}}}); err == nil {
			t.Errorf("%s, not the track's worktree, was reviewed", other)
		}
	}
	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "gone", Dir: worktree, Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "x"}}}}); err == nil {
		t.Error("a track that isn't there was reviewed")
	}
}

// A reviewer runs in a train's session with a header of its own, whose
// badge follows its status; End train ends it.
func TestReviewerTrain(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	worktree := t.TempDir()
	if err := f.store.AddTrack(ctx, track.Track{ID: "r", Kind: track.Work, Name: "r", Engine: "claude",
		Repos: []track.Repo{{Name: "api", Worktree: worktree}}}); err != nil {
		t.Fatal(err)
	}
	f.windows.windows = append(f.windows.windows, trackwin.Info{Window: "@r", Track: "r", Name: "r"})
	panes := &fakeReviewers{}
	f.svc.Reviewers = panes
	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: worktree, Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "+x\n"}}}}); err != nil {
		t.Fatal(err)
	}
	handle := reviewHandle(f.svc.reviewDir("r"))
	spec := panes.specs[0]
	if spec.Track != "r" || spec.Handle != handle || spec.Role != trackwin.RoleReviewer ||
		spec.Header != actions.TrainHeader(handle, "Reviewer", trackwin.TrainBadgeOption) {
		t.Errorf("spec %+v; want the reviewer's train, %s, with its header", spec, handle)
	}
	session := panes.state.Session
	if got := panes.badges[session]; got != actions.Badges(track.Active) {
		t.Errorf("badge %q as it starts, want active", got)
	}
	if b := f.windows.buttons["@r"]; b != "" {
		t.Errorf("buttons %q; the footer has no reviewer buttons any more", b)
	}
	run, _ := os.ReadFile(filepath.Join(f.svc.reviewDir("r"), "run"))
	if err := f.svc.ReviewReply(ctx, "r", string(run), "Report.\nREVIEW OUTCOME: pass"); err != nil {
		t.Fatal(err)
	}
	panes.badgeErr = errors.New("the session is starting")
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	panes.badgeErr = nil
	if err := f.svc.SyncTrains(ctx); err != nil {
		t.Fatal(err)
	}
	if got := panes.badges[session]; got != actions.Badges(track.Done, train.Pass) {
		t.Errorf("badge %q once the report is in, want done and pass, written again after a failed write", got)
	}
	panes.state.Exited = true
	trains, err := f.svc.Trains(ctx, "r")
	if err != nil || len(trains) != 1 || trains[0].Status != track.Done {
		t.Errorf("trains %+v, %v; a reviewer that exited after its report is done", trains, err)
	}

	if _, err := f.svc.EndTrain(ctx, "r", "reviewer"); err != nil {
		t.Fatal(err)
	}
	if panes.state.Pane != "" || len(panes.closed) != 2 {
		t.Errorf("reviewer %+v after End train; want it ended", panes.state)
	}
	if _, err := os.Stat(f.svc.reviewDir("r")); !os.IsNotExist(err) {
		t.Errorf("the ended reviewer's run is left: %v", err)
	}
}

func TestReviewerStates(t *testing.T) {
	panes := []tmux.Pane{{ID: "%3"}, {ID: "%4", Role: trackwin.RoleReviewer, Track: "b", Session: "s", Dead: true},
		{ID: "%5", Role: trackwin.RoleReviewer}}
	got := reviewerStates(panes)
	if len(got) != 1 || got["b"] != (ReviewerState{Pane: "%4", Session: "s", Exited: true}) {
		t.Errorf("reviewerStates = %+v; want b's", got)
	}
}

// A question reaches the reviewer once it has reported, and its next
// reply is the answer; --wait then waits for that answer.
func TestAskReview(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	worktree := t.TempDir()
	if err := f.store.AddTrack(ctx, track.Track{ID: "r", Kind: track.Work, Name: "r", Engine: "claude",
		Repos: []track.Repo{{Name: "api", Worktree: worktree}}}); err != nil {
		t.Fatal(err)
	}
	f.windows.windows = append(f.windows.windows, trackwin.Info{Window: "@r", Track: "r", Name: "r"})
	panes := &fakeReviewers{}
	f.svc.Reviewers = panes
	f.svc.Settings = func() (settings.Settings, error) { return settings.Settings{Engines: f.engines}, nil }

	if err := f.svc.AskReview(ctx, "r", "Why?"); err == nil {
		t.Error("a track without a reviewer was asked")
	}
	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: worktree, Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "+x\n"}}}}); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.AskReview(ctx, "r", "Why?"); err == nil || !strings.Contains(err.Error(), "hasn't reported") {
		t.Errorf("a reviewer still on its review was asked: %v", err)
	}
	run, _ := os.ReadFile(filepath.Join(f.svc.reviewDir("r"), "run"))
	reply := func(text string) {
		t.Helper()
		if err := f.svc.ReviewReply(ctx, "r", string(run), text); err != nil {
			t.Fatal(err)
		}
	}
	reply("Report.\nREVIEW OUTCOME: blocked")
	reply("Talk in the pane, before any question.")
	for _, bad := range []string{"", "  ", "line\nbreak", "esc\x1b[A"} {
		if err := f.svc.AskReview(ctx, "r", bad); err == nil {
			t.Errorf("question %q was asked", bad)
		}
	}
	panes.state.Attached = true
	if err := f.svc.AskReview(ctx, "r", "Why?"); err == nil || !strings.Contains(err.Error(), "overlay open") {
		t.Errorf("a reviewer in an open overlay was asked: %v", err)
	}
	panes.state.Attached = false
	panes.state.Exited = true
	if err := f.svc.AskReview(ctx, "r", "Why?"); err == nil || !strings.Contains(err.Error(), "exited") {
		t.Errorf("a reviewer whose agent exited was asked: %v", err)
	}
	panes.state.Exited = false
	if err := f.svc.AskReview(ctx, "r", "  Is finding 1 real?  "); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(f.svc.reviewDir("r"), "question.md")); !strings.HasPrefix(string(b), questionLead) {
		t.Errorf("question.md %q", b)
	}
	if len(panes.typed) != 1 || panes.typed[0] != "%9 "+questionLead+"Is finding 1 real?" {
		t.Errorf("typed %q; want the question in the reviewer's pane", panes.typed)
	}
	if err := f.svc.AskReview(ctx, "r", "And 2?"); err == nil {
		t.Error("a second question went in before the first was answered")
	}
	reply("Yes: the cache is never cleared.\nREVIEW OUTCOME: blocked")
	reply("Later talk.")
	answer, err := f.svc.WaitReview(ctx, "r")
	if err != nil || answer != "Yes: the cache is never cleared.\nREVIEW OUTCOME: blocked" {
		t.Errorf("answer %q, %v; want the first reply after the question", answer, err)
	}

	f.svc.Settings = func() (settings.Settings, error) {
		return settings.Settings{Engines: f.engines, Agents: settings.Agents{Reviewer: &settings.Agent{Engine: "cursor"}}}, nil
	}
	if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: worktree, Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "+x\n"}}}}); err != nil {
		t.Fatal(err)
	}
	run, _ = os.ReadFile(filepath.Join(f.svc.reviewDir("r"), "run"))
	reply("Report.\nREVIEW OUTCOME: pass")
	if err := f.svc.AskReview(ctx, "r", "What about tests?"); err != nil {
		t.Fatal(err)
	}
	if len(panes.respawned) != 1 {
		t.Fatalf("respawned %q; want Cursor's pane started again on the question", panes.respawned)
	}
	for _, want := range []string{"%9 " + worktree, "-p", "--resume", "What about tests?", "answer.out", "--answer", "exec"} {
		if !strings.Contains(panes.respawned[0], want) {
			t.Errorf("Cursor's turn on the question lacks %s: %s", want, panes.respawned[0])
		}
	}
}

// A Review track narrows to its repo's Code owner; a Work track's
// pre-push review checks its whole change.
func TestReviewOwner(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	repos, err := f.store.Repos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	api := repos[0]
	api.CodeOwner = "@org/api"
	if _, err := f.store.UpdateRepo(ctx, api); err != nil {
		t.Fatal(err)
	}
	worktree := t.TempDir()
	for _, tr := range []track.Track{
		{ID: "rev", Kind: track.Review, Name: "rev", Engine: "claude", Repos: []track.Repo{{RepoID: api.ID, Name: "api", Worktree: worktree}}},
		{ID: "work", Kind: track.Work, Name: "work", Engine: "claude", Repos: []track.Repo{{RepoID: api.ID, Name: "api", Worktree: worktree}}},
	} {
		if err := f.store.AddTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	if owner, err := f.svc.ReviewOwner(ctx, "rev", worktree); err != nil || owner != "@org/api" {
		t.Errorf("a Review track: %q, %v; want its repo's Code owner", owner, err)
	}
	if owner, err := f.svc.ReviewOwner(ctx, "work", worktree); err != nil || owner != "" {
		t.Errorf("a Work track: %q, %v; want none", owner, err)
	}
	if _, err := f.svc.ReviewOwner(ctx, "rev", t.TempDir()); err == nil {
		t.Error("a folder outside the track's worktrees was answered")
	}
}

// A Claude reviewer stuck on its trust question is action required, and
// the user is told, once a run.
func TestReviewerAsksTrustIsTold(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	worktree := t.TempDir()
	if err := f.store.AddTrack(ctx, track.Track{ID: "r", Kind: track.Work, Name: "rate-bug", Engine: "claude",
		Repos: []track.Repo{{Name: "api", Worktree: worktree}}}); err != nil {
		t.Fatal(err)
	}
	f.windows.windows = append(f.windows.windows, trackwin.Info{Window: "@r", Track: "r", Name: "r"})
	panes := &fakeReviewers{}
	f.svc.Reviewers = panes
	var told []Notice
	f.svc.Notify = func(n Notice) { told = append(told, n) }
	start := func() {
		t.Helper()
		if _, err := f.svc.StartReview(ctx, ReviewRequest{ID: "r", Dir: worktree, Packet: review.Packet{Diffs: []review.FileDiff{{Path: "a", Text: "+x\n"}}}}); err != nil {
			t.Fatal(err)
		}
	}
	tick := func() {
		t.Helper()
		if err := f.svc.SyncTrains(ctx); err != nil {
			t.Fatal(err)
		}
	}
	start()
	tick()
	if len(told) != 0 {
		t.Fatalf("told %+v about a reviewer at work", told)
	}
	panes.screen = " Quick safety check: Is this a project you created or one you trust?\n ❯ No, exit\n   Yes, I trust this folder"
	tick()
	tick()
	handle := reviewHandle(f.svc.reviewDir("r"))
	if len(told) != 1 || told[0].Event != settings.NotifyActionRequired || !strings.Contains(told[0].Title, "@"+handle) ||
		!strings.Contains(told[0].Title, "rate-bug") {
		t.Fatalf("told %+v; want the track needing the user once, naming the reviewer", told)
	}
	if got := panes.badges[panes.state.Session]; got != actions.Badges(track.ActionRequired) {
		t.Errorf("badge %q, want action required", got)
	}
	start()
	tick()
	if len(told) != 2 {
		t.Errorf("told %d times; a new review asking again is told again", len(told))
	}
}
