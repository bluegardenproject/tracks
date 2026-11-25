package tracks

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
