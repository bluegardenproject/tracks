package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestPRs(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	now := time.UnixMilli(1_790_000_000_000)
	for _, id := range []string{"a", "b"} {
		if err := s.AddTrack(ctx, track.Track{ID: id, Kind: track.Work, Name: id, Engine: "claude", CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	web, _ := track.ParsePR("https://github.com/acme/web/pull/7")
	api, _ := track.ParsePR("https://github.com/acme/api/pull/3")

	if added, err := s.AddPR(ctx, "a", web, now); err != nil || !added {
		t.Fatalf("AddPR = %v, %v", added, err)
	}
	merged := web
	merged.State, merged.CheckedAt = track.PRMerged, now.Add(time.Minute)
	if err := s.SavePR(ctx, "a", merged, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if added, err := s.AddPR(ctx, "a", web, now.Add(2*time.Minute)); err != nil || added {
		t.Errorf("adding a known PR again = %v, %v; want it left as it is", added, err)
	}
	if err := s.SavePR(ctx, "a", api, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddPR(ctx, "b", web, now); err != nil {
		t.Fatal(err)
	}

	a, err := s.Track(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.PRs) != 2 || a.PRs[0].URL != web.URL || a.PRs[0].State != track.PRMerged ||
		!a.PRs[0].CheckedAt.Equal(merged.CheckedAt) || a.PRs[1] != api {
		t.Errorf("a's PRs = %+v; want web merged, then api open", a.PRs)
	}
	unsettled, err := s.UnsettledPRs(ctx)
	if err != nil || len(unsettled) != 2 || unsettled[0].TrackID != "a" || unsettled[0].PR != api ||
		unsettled[1].TrackID != "b" || unsettled[1].URL != web.URL {
		t.Errorf("UnsettledPRs = %+v, %v; want a's api and b's web", unsettled, err)
	}
	if open, _ := s.OpenTracks(ctx); len(open) != 2 || len(open[1].PRs) != 1 {
		t.Errorf("OpenTracks carry their PRs: %+v", open)
	}
}
