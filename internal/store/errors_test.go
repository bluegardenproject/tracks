package store

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/track"
)

func TestTrackErrors(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	if err := s.AddTrack(ctx, track.Track{ID: "a", Kind: track.Work, Name: "a", Engine: "claude", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []track.Failure{{Kind: track.SetupError, Subject: "api", Code: 1}, {Kind: track.ServerError, Subject: "api/web", Code: 2}, {Kind: track.ServerError, Subject: "api/web", Code: 3}} {
		if err := s.AddTrackError(ctx, "a", e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Track(ctx, "a")
	slices.SortFunc(got.Failures, func(a, b track.Failure) int { return strings.Compare(b.Kind, a.Kind) })
	want := []track.Failure{{Kind: track.SetupError, Subject: "api", Code: 1}, {Kind: track.ServerError, Subject: "api/web", Code: 3}}
	if err != nil || !slices.Equal(got.Failures, want) {
		t.Fatalf("errors = %+v, %v; want %+v (the second web crash replacing the first)", got.Failures, err, want)
	}
	if found, err := s.ClearTrackErrors(ctx, "a", track.ServerError, "api/web"); !found || err != nil {
		t.Errorf("clearing web: %v, %v", found, err)
	}
	if found, _ := s.ClearTrackErrors(ctx, "a", track.ServerError, "api/web"); found {
		t.Error("clearing web twice found one")
	}
	if found, _ := s.ClearTrackErrors(ctx, "a", "", ""); !found {
		t.Error("clearing all didn't find the setup error")
	}
	if got, _ := s.Track(ctx, "a"); len(got.Failures) != 0 {
		t.Errorf("errors after clearing all: %+v", got.Failures)
	}
}
