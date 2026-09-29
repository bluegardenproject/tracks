package tracks

import (
	"context"
	"errors"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestStationFilter(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	var ids []string
	for _, prompt := range []string{"one", "two", "three"} {
		got, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: prompt}, func(string) {})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, got.Track.ID)
	}
	if err := f.svc.End(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Archive(ctx, ids[1], false); err != nil {
		t.Fatal(err)
	}

	listed, filter, err := f.svc.Station(ctx)
	if err != nil || filter.On() || len(listed) != 2 {
		t.Fatalf("Station unfiltered = %d tracks, %+v, %v; want the two open ones", len(listed), filter, err)
	}

	var p Problem
	if err := f.svc.SetFilter(ctx, track.Filter{Started: track.Between, From: "2026-09-02", To: "2026-09-01"}); !errors.As(err, &p) {
		t.Errorf("an invalid filter: %v, want a problem", err)
	}
	want := track.Filter{Statuses: []string{"active"}, Started: track.Today}
	if err := f.svc.SetFilter(ctx, want); err != nil {
		t.Fatal(err)
	}
	listed, filter, err = f.svc.Station(ctx)
	if err != nil || filter.String() != want.String() || len(listed) != 2 {
		t.Fatalf("Station filtered = %+v, %+v, %v", listed, filter, err)
	}
	// Newest first, with their windows.
	if listed[0].ID != ids[2] || listed[1].ID != ids[0] || listed[0].Number == 0 || listed[0].Window == "" {
		t.Errorf("filtered = %+v, want three then one, with their windows", listed)
	}

	if err := f.svc.SetFilter(ctx, track.Filter{Archived: true}); err != nil {
		t.Fatal(err)
	}
	if listed, _, err := f.svc.Station(ctx); err != nil || len(listed) != 1 || listed[0].ID != ids[1] || listed[0].Number != 0 {
		t.Errorf("archived only = %+v, %v; want two, without a window", listed, err)
	}
	if err := f.svc.Unarchive(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	if listed, _, _ := f.svc.Station(ctx); len(listed) != 0 {
		t.Errorf("after Unarchive, archived only = %+v", listed)
	}

	if err := f.svc.SetFilter(ctx, track.Filter{}); err != nil {
		t.Fatal(err)
	}
	if listed, filter, _ := f.svc.Station(ctx); filter.On() || len(listed) != 3 {
		t.Errorf("cleared: %d tracks, %+v; want all three", len(listed), filter)
	}
}
