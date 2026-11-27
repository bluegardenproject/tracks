package tracks

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/relay"
	"github.com/bluegardenproject/tracks/internal/side"
	"github.com/bluegardenproject/tracks/internal/track"
)

// sideFixture is a fixture with a main track and one side track of it.
func sideFixture(t *testing.T) (*fixture, track.Track, track.Track) {
	t.Helper()
	ctx := context.Background()
	f := newFixture(t)
	main, err := f.svc.Create(ctx, Request{Kind: track.Work, Name: "api-cache", Repos: []string{"api"}, Prompt: "Cache the API"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.Create(ctx, Request{Kind: track.Work, Name: "relay-store", Prompt: "Store the relay\nin SQLite",
		Parent: main.Track.ID, Side: true, NoDraft: true}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	return f, main.Track, st.Track
}

func TestSideNew(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	saved, err := f.store.Track(ctx, st.ID)
	switch {
	case err != nil:
		t.Fatal(err)
	case saved.Parent != main.ID || !strings.HasPrefix(saved.Side, side.Prefix+"-") || !side.Is(saved):
		t.Errorf("side track saved with parent %q and handle %q; want %s and a side handle", saved.Parent, saved.Side, main.ID)
	case len(saved.Repos) != 1 || saved.Repos[0].Name != "api" || saved.Kind != track.Work:
		t.Errorf("side track is a %s track on %+v; want a Work track on its main track's api", saved.Kind, saved.Repos)
	}
	msgs, err := f.store.RelayUnread(ctx, main.ID, relay.TrackHandle)
	if err != nil || len(msgs) != 1 || msgs[0].Body != "@"+st.Side+" joined: side track relay-store, Store the relay" {
		t.Errorf("main track's relay holds %+v, %v; want the join notice", msgs, err)
	}

	var problem Problem
	for _, tc := range []struct {
		req Request
		why string
	}{
		{Request{Kind: track.Work, Prompt: "More", Parent: st.ID, Side: true}, "starts no side tracks"},
		{Request{Kind: track.Plan, Prompt: "Plan", Parent: main.ID, Side: true}, "a Work track"},
		{Request{Kind: track.Work, Prompt: "Orphan", Side: true}, "from inside a track"},
		{Request{Kind: track.Work, Prompt: "Gone", Parent: "20261124-000000-gone00", Side: true}, "gone"},
	} {
		tc.req.NoDraft = true
		if _, err := f.svc.Create(ctx, tc.req, func(string) {}); !errors.As(err, &problem) || !strings.Contains(err.Error(), tc.why) {
			t.Errorf("%+v: %v; want a Problem saying %q", tc.req, err, tc.why)
		}
	}
	if sides, err := f.svc.SideTracks(ctx, main.ID); err != nil || len(sides) != 1 || sides[0].ID != st.ID {
		t.Errorf("side tracks %+v, %v; want relay-store alone", sides, err)
	}
}

func TestSideRelay(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	if _, err := f.svc.RelayRead(ctx, main.ID, relay.TrackHandle); err != nil {
		t.Fatal(err)
	}
	report, err := f.svc.RelaySend(ctx, st.ID, "@track", relay.Report, "PR #231 is ready.", relay.Fields{})
	if err != nil {
		t.Fatal(err)
	}
	if report.From != st.Side || report.To != relay.TrackHandle {
		t.Errorf("a side track sent as %s to %s; want as %s to the main track", report.From, report.To, st.Side)
	}
	if msgs, err := f.svc.RelayRead(ctx, main.ID, relay.TrackHandle); err != nil || len(msgs) != 1 || msgs[0].ID != report.ID {
		t.Errorf("main track read %+v, %v; want the side track's report", msgs, err)
	}

	question, err := f.svc.RelaySend(ctx, main.ID, side.Prefix, relay.Question, "Which table?", relay.Fields{})
	if err != nil {
		t.Fatal(err)
	}
	if question.To != st.Side {
		t.Errorf("sent to %s; want %s, the one side track", question.To, st.Side)
	}
	if due, err := f.svc.dueMessages(ctx, st, 0, f.svc.now().Add(time.Minute), pointerGrace); err != nil || len(due) != 1 || due[0].ID != question.ID {
		t.Errorf("due for the side track %+v, %v; want its main track's question", due, err)
	}
	msgs, err := f.svc.RelayRead(ctx, st.ID, relay.TrackHandle)
	if err != nil || len(msgs) != 2 || msgs[1].ID != question.ID {
		t.Fatalf("side track read %+v, %v; want the join notice and the question", msgs, err)
	}
	if m, err := f.svc.RelayShow(ctx, st.ID, relay.TrackHandle, question.ID); err != nil || m.Body != "Which table?" {
		t.Errorf("side track showed %+v, %v", m, err)
	}
	other, err := f.store.AddRelayMessage(ctx, main.ID, relay.Message{From: "reviewer-1", To: relay.TrackHandle, Kind: relay.Report, Body: "Fine."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.RelayShow(ctx, st.ID, relay.TrackHandle, other.ID); err == nil {
		t.Error("a side track showed a message of its main track's relay that isn't for it")
	}

	var problem Problem
	if _, err := f.svc.RelaySend(ctx, st.ID, side.Prefix, relay.Question, "Hi?", relay.Fields{}); !errors.As(err, &problem) {
		t.Errorf("a side track sent to a side track: %v, want a Problem", err)
	}
	who, err := f.svc.RelayWho(ctx, main.ID)
	if err != nil || len(who) != 2 || who[1].Handle != st.Side {
		t.Errorf("main track's relay has %+v, %v; want the track and the side track", who, err)
	}
	who, err = f.svc.RelayWho(ctx, st.ID)
	if err != nil || len(who) != 2 || who[0].Handle != relay.TrackHandle || !strings.Contains(who[0].Agent, "api-cache") || who[1].Handle != st.Side {
		t.Errorf("side track sees %+v, %v; want its main track and itself", who, err)
	}
}

func TestMainTrackWaitsForItsSideTracks(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	var problem Problem
	if err := f.svc.End(ctx, main.ID); !errors.As(err, &problem) || !strings.Contains(err.Error(), "relay-store (running)") {
		t.Fatalf("ending a main track with a side track running: %v, want a Problem naming it", err)
	}
	pr, _ := track.ParsePR("https://github.com/o/api/pull/231")
	if _, err := f.store.AddPR(ctx, st.ID, pr, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.End(ctx, st.ID); err != nil {
		t.Fatal(err)
	}
	unread, err := f.store.RelayUnread(ctx, main.ID, relay.TrackHandle)
	if err != nil || len(unread) == 0 || unread[len(unread)-1].Body != "@"+st.Side+" left: it ended." {
		t.Errorf("main track's relay holds %+v, %v; want the side track's left notice last", unread, err)
	}
	if _, err := f.svc.RelaySend(ctx, main.ID, st.Side, relay.Question, "Still there?", relay.Fields{}); !errors.As(err, &problem) {
		t.Errorf("sending to an ended side track: %v, want a Problem", err)
	}
	if err := f.svc.End(ctx, main.ID); !errors.As(err, &problem) || !strings.Contains(err.Error(), "PR #231 open") {
		t.Fatalf("ending a main track whose side track's PR is open: %v, want a Problem naming it", err)
	}
	pr.State = track.PRMerged
	if _, err := f.store.SavePR(ctx, st.ID, pr, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.End(ctx, main.ID); err != nil {
		t.Errorf("ending a main track whose side tracks are settled: %v", err)
	}
}

func TestSettledGuardsArchiveAndDerail(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	// The main track's window closed some other way than End.
	if err := f.svc.Report(ctx, main.ID, track.Ended); err != nil {
		t.Fatal(err)
	}
	var problem Problem
	if _, err := f.svc.Archive(ctx, main.ID, true); !errors.As(err, &problem) {
		t.Errorf("archiving a main track with a side track running: %v, want a Problem", err)
	}
	if _, err := f.svc.Derail(ctx, main.ID, true); !errors.As(err, &problem) {
		t.Errorf("derailing a main track with a side track running: %v, want a Problem", err)
	}
	if err := f.svc.End(ctx, st.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Archive(ctx, main.ID, true); err != nil {
		t.Errorf("archiving a main track whose side tracks are settled: %v", err)
	}
}

func TestSideNewRefusals(t *testing.T) {
	ctx := context.Background()
	f, main, _ := sideFixture(t)
	plan, err := f.svc.Create(ctx, Request{Kind: track.Plan, Name: "plan", Prompt: "Plan it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	var problem Problem
	if _, err := f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "Build it", Parent: plan.Track.ID, Side: true, NoDraft: true},
		func(string) {}); !errors.As(err, &problem) || !strings.Contains(err.Error(), "has no repos") {
		t.Errorf("a side track of a main track without repos, none named: %v, want a Problem", err)
	}
	if _, err := f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "Build it", Parent: plan.Track.ID, Side: true, NoDraft: true,
		Repos: []string{"web"}}, func(string) {}); err != nil {
		t.Errorf("a side track on a repo named: %v", err)
	}

	_, release, err := f.svc.hold(ctx, main.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "More", Parent: main.ID, Side: true, NoDraft: true},
		func(string) {}); !errors.As(err, &problem) || !strings.Contains(err.Error(), "busy") {
		t.Errorf("a side track of a main track being ended: %v, want a Problem", err)
	}
	release()
	done, err := f.svc.makingSide(main.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.End(ctx, main.ID); !errors.As(err, &problem) || !strings.Contains(err.Error(), "starting a side track") {
		t.Errorf("ending a main track while a side track is being made: %v, want a Problem", err)
	}
	done()

	if err := f.svc.Report(ctx, main.ID, track.Ended); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "Late", Parent: main.ID, Side: true, NoDraft: true},
		func(string) {}); !errors.As(err, &problem) || !strings.Contains(err.Error(), "has ended") {
		t.Errorf("a side track of an ended main track: %v, want a Problem", err)
	}
	if _, err := f.svc.Lost(ctx, main.ID); !errors.As(err, &problem) {
		t.Errorf("checking an ended main track with a side track running for lost work: %v, want a Problem", err)
	}
}

func TestAutoArchiveSkipsAMainTrackWaiting(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	f.history.AutoArchive = true
	if err := f.svc.Report(ctx, main.ID, track.Ended); err != nil {
		t.Fatal(err)
	}
	later := f.svc.now().Add(ArchiveAfter + time.Hour)
	f.svc.Now = func() time.Time { return later }
	if names, err := f.svc.AutoArchive(ctx); err != nil || len(names) != 0 {
		t.Errorf("auto-archive took %v, %v; want nothing while a side track runs", names, err)
	}
	if err := f.svc.End(ctx, st.ID); err != nil {
		t.Fatal(err)
	}
	f.svc.Now = func() time.Time { return later.Add(ArchiveAfter + time.Hour) }
	if names, err := f.svc.AutoArchive(ctx); err != nil || len(names) != 2 {
		t.Errorf("auto-archive took %v, %v; want the main track and its side track", names, err)
	}
}

func TestSideTrackReadsBothRelays(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	if _, err := f.svc.RelayRead(ctx, st.ID, relay.TrackHandle); err != nil {
		t.Fatal(err)
	}
	question, err := f.svc.RelaySend(ctx, main.ID, st.Side, relay.Question, "Which table?", relay.Fields{})
	if err != nil {
		t.Fatal(err)
	}
	report, err := f.store.AddRelayMessage(ctx, st.ID, relay.Message{From: "reviewer-1", To: relay.TrackHandle, Kind: relay.Report, Body: "Fine."})
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := f.svc.RelayWait(ctx, st.ID, relay.TrackHandle)
	if err != nil || len(msgs) != 2 || msgs[0].ID != question.ID || msgs[1].ID != report.ID {
		t.Fatalf("side track read %+v, %v; want its main track's question, then its reviewer's report", msgs, err)
	}
	if m, err := f.svc.RelayShow(ctx, st.ID, relay.TrackHandle, report.ID); err != nil || m.Body != "Fine." {
		t.Errorf("show from its own relay: %+v, %v", m, err)
	}
	if msgs, err := f.svc.RelayRead(ctx, st.ID, relay.TrackHandle); err != nil || len(msgs) != 0 {
		t.Errorf("read again %+v, %v; want nothing", msgs, err)
	}
}

func TestRelayWaitSleepsThroughNotices(t *testing.T) {
	ctx := context.Background()
	f, main, _ := sideFixture(t)
	ctx, cancel := context.WithTimeout(ctx, 3*relayPoll)
	defer cancel()
	// Only the side track's join notice is unread.
	if msgs, err := f.svc.RelayWait(ctx, main.ID, relay.TrackHandle); err == nil {
		t.Errorf("a notice woke relay wait: %+v", msgs)
	}
}

func TestSideTrackLeavesWhenTracksStops(t *testing.T) {
	ctx := context.Background()
	f, main, st := sideFixture(t)
	if err := f.svc.Report(ctx, st.ID, track.Interrupted); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Report(ctx, st.ID, track.Resumed); err != nil {
		t.Fatal(err)
	}
	msgs, err := f.store.RelayUnread(ctx, main.ID, relay.TrackHandle)
	if err != nil || len(msgs) != 3 || msgs[1].Body != "@"+st.Side+" left: Tracks stopped." ||
		!strings.HasPrefix(msgs[2].Body, "@"+st.Side+" joined:") {
		t.Errorf("main track's relay holds %+v, %v; want joined, left as Tracks stopped, joined again", msgs, err)
	}
}

func TestSideTrackPromptNamesItsMainTrack(t *testing.T) {
	ctx := context.Background()
	f, _, st := sideFixture(t)
	spec := f.engine.spec
	if spec.Side == nil || spec.Side.Main != "api-cache" || spec.Side.Handle != st.Side {
		t.Errorf("side track started with %+v; want its main track api-cache and its handle", spec.Side)
	}
	if len(spec.DraftPRs) != 1 || spec.DraftPRs[0] != "api" {
		t.Errorf("side track's repos keeping drafts %v; want api, by its rule", spec.DraftPRs)
	}
	got, err := f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "Plain", Repos: []string{"web"}}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if f.engine.spec.Side != nil || got.DraftPRs != nil {
		t.Errorf("a track that isn't a side track started with %+v, drafts %v", f.engine.spec.Side, got.DraftPRs)
	}
}

func TestSideTrackPromptNamesItsMainTrackAsShown(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	main, err := f.svc.Create(ctx, Request{Kind: track.Work, Name: "api-cache", Title: "API cache", Repos: []string{"api"}, Prompt: "Cache the API"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.Create(ctx, Request{Kind: track.Work, Prompt: "Store the relay", Parent: main.Track.ID, Side: true, NoDraft: true}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(st.DraftPRs, []string{"api"}) {
		t.Errorf("side track made with repos keeping drafts %v; want api, by its rule", st.DraftPRs)
	}
	if s := f.engine.spec.Side; s == nil || s.Main != "API cache" || s.Handle != st.Track.Side {
		t.Errorf("side track started with %+v; want its main track as shown, API cache, and its handle", s)
	}

	// A side track whose session never started starts fresh: its prompt
	// names its main track again.
	f.engine.unstarted = true
	f.engine.spec = agents.Spec{}
	f.windows.windows[1].Exit = "1"
	if _, err := f.svc.CheckExits(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Restart(ctx, st.Track.ID, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if s := f.engine.spec.Side; f.engine.spec.Resume || s == nil || s.Main != "API cache" || s.Handle != st.Track.Side {
		t.Errorf("restarted side track with %+v, resume %v; want a fresh start naming API cache", s, f.engine.spec.Resume)
	}
}
