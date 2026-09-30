package store

import (
	"context"
	"errors"
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
	if was, err := s.SavePR(ctx, "a", merged, now.Add(time.Minute)); err != nil || was != track.PROpen {
		t.Fatalf("saving a merged PR = %q, %v; want it was open", was, err)
	}
	if added, err := s.AddPR(ctx, "a", web, now.Add(2*time.Minute)); err != nil || added {
		t.Errorf("adding a known PR again = %v, %v; want it left as it is", added, err)
	}
	if was, err := s.SavePR(ctx, "a", api, now.Add(3*time.Minute)); err != nil || was != "" {
		t.Fatalf("saving a new PR = %q, %v; want no state before", was, err)
	}
	checked := merged
	checked.CheckedAt = now.Add(4 * time.Minute)
	if was, err := s.SavePR(ctx, "a", checked, now.Add(4*time.Minute)); err != nil || was != track.PRMerged {
		t.Errorf("saving a PR checked again = %q, %v; want it was merged", was, err)
	}
	merged.CheckedAt = checked.CheckedAt
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

func TestDeleteTrack(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	now := time.UnixMilli(1_790_000_000_000)
	for _, id := range []string{"a", "b"} {
		tr := track.Track{ID: id, Kind: track.Work, Name: id, Engine: "claude", CreatedAt: now,
			Repos: []track.Repo{{Name: "web", Path: "/src/web", Branch: "tracks/" + id}}}
		if err := s.AddTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
		pr, _ := track.ParsePR("https://github.com/acme/web/pull/7")
		if _, err := s.AddPR(ctx, id, pr, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeleteTrack(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Track(ctx, "a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("reading a deleted track: %v", err)
	}
	for _, table := range []string{"track_repos", "track_prs"} {
		var n int
		if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE track_id = 'a'").Scan(&n); err != nil || n != 0 {
			t.Errorf("%s rows left: %d, %v", table, n, err)
		}
	}
	if b, err := s.Track(ctx, "b"); err != nil || len(b.Repos) != 1 || len(b.PRs) != 1 {
		t.Errorf("the other track = %+v, %v", b, err)
	}
	if err := s.DeleteTrack(ctx, "a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleting it again: %v", err)
	}
}
