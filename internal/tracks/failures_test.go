package tracks

import (
	"context"
	"testing"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
)

func failures(t *testing.T, f *fixture, id string) []track.Failure {
	t.Helper()
	got, err := f.store.Track(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return got.Failures
}

func TestFailuresShowUntilCleared(t *testing.T) {
	f := newFixture(t)
	withServers(t, f, "api", []store.DevServer{{Name: "web", Command: "pnpm dev", PortMode: store.PortAssigned}})
	ctx := context.Background()
	tr := work(t, f, "api")

	if err := f.svc.ReportExit(ctx, tr.ID, track.ServerError, "api/web", 0); err != nil || len(failures(t, f, tr.ID)) != 0 {
		t.Fatalf("a clean exit: %v, %+v; want nothing recorded", err, failures(t, f, tr.ID))
	}
	if err := f.svc.ReportExit(ctx, tr.ID, "nope", "x", 1); !isProblem(err) {
		t.Errorf("an unknown kind: %v", err)
	}
	if err := f.svc.ReportExit(ctx, "gone", track.ServerError, "api/web", 1); !isProblem(err) {
		t.Errorf("a track that's gone: %v; want a problem", err)
	}

	for _, e := range []track.Failure{{Kind: track.ServerError, Subject: "api/web", Code: 1}, {Kind: track.SetupError, Subject: "api", Code: 2}} {
		if err := f.svc.ReportExit(ctx, tr.ID, e.Kind, e.Subject, e.Code); err != nil {
			t.Fatal(err)
		}
	}
	if got := failures(t, f, tr.ID); len(got) != 2 || track.FailureStatus(got) != track.BothErrored {
		t.Fatalf("failures = %+v", got)
	}
	if _, err := f.svc.Up(ctx, tr.ID, "web"); err != nil {
		t.Fatal(err)
	}
	if got := failures(t, f, tr.ID); len(got) != 1 || got[0].Kind != track.SetupError {
		t.Errorf("after starting web again: %+v; want only the setup error", got)
	}
	_ = f.svc.ReportExit(ctx, tr.ID, track.ServerError, "api/web", 1)
	if _, err := f.svc.Down(ctx, tr.ID, "web"); err != nil {
		t.Fatal(err)
	}
	if got := failures(t, f, tr.ID); len(got) != 1 {
		t.Errorf("after stopping web: %+v; want its error cleared", got)
	}
	if err := f.svc.DismissFailures(ctx, tr.ID); err != nil || len(failures(t, f, tr.ID)) != 0 {
		t.Errorf("after dismissing: %v, %+v", err, failures(t, f, tr.ID))
	}
}

func TestStartingASetupAgainClearsItsFailure(t *testing.T) {
	f := newFixture(t)
	setups, _ := withSetup(t, f, "api", "pnpm install", false)
	ctx := context.Background()
	tr := work(t, f, "api")
	setups.setState("api", "failed 1")
	_ = f.svc.ReportExit(ctx, tr.ID, track.SetupError, "api", 1)
	if _, err := f.svc.StartSetup(ctx, tr.ID, true); err != nil {
		t.Fatal(err)
	}
	if got := failures(t, f, tr.ID); len(got) != 0 {
		t.Errorf("after running the setup again: %+v", got)
	}
}
