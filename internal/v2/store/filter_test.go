package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestFilterIsKept(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tracks.db")
	s := open(t, path)
	if f, err := s.Filter(ctx); err != nil || f.On() {
		t.Fatalf("Filter at first = %+v, %v; want none", f, err)
	}
	want := track.Filter{Statuses: []string{"done", "closed"}, PRStatuses: []string{"none"}, Archived: true,
		Started: track.Between, From: "2026-09-01", To: "2026-09-20"}
	if err := s.SetFilter(ctx, want); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFilter(ctx, track.Filter{Started: track.Today}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFilter(ctx, want); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s = open(t, path)
	if got, err := s.Filter(ctx); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Filter after reopening = %+v, %v; want %+v", got, err, want)
	}
	if err := s.SetFilter(ctx, track.Filter{}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Filter(ctx); err != nil || got.On() {
		t.Errorf("Filter after clearing = %+v, %v", got, err)
	}
}

func TestFilteredTracks(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	now := time.Date(2026, 9, 29, 18, 0, 0, 0, time.Local)
	add := func(id string, created time.Time, st track.State, prs ...track.PRState) {
		t.Helper()
		if err := s.AddTrack(ctx, track.Track{ID: id, Kind: track.Work, Name: id, Engine: "claude", CreatedAt: created, State: st}); err != nil {
			t.Fatal(err)
		}
		for i, state := range prs {
			pr, _ := track.ParsePR("https://github.com/acme/api/pull/" + string(rune('1'+i)))
			pr.State = state
			if _, err := s.AddPR(ctx, id, pr, now); err != nil {
				t.Fatal(err)
			}
		}
	}
	ended := now.Add(-time.Hour)
	add("active", now.Add(-2*time.Hour), track.State{})
	add("waiting", now.AddDate(0, 0, -1), track.State{Waiting: true}, track.PROpen)
	add("done", now.AddDate(0, 0, -3), track.State{ClosedAt: ended}, track.PRMerged)
	add("cleaned", now.AddDate(0, 0, -10), track.State{ClosedAt: ended, CleanedAt: ended})
	add("archived", now.AddDate(0, 0, -40), track.State{ClosedAt: ended, CleanedAt: ended, ArchivedAt: ended}, track.PRClosed)
	add("failed", now.AddDate(0, -2, 0), track.State{Exit: track.ExitFailed, Waiting: true})
	add("exited", now.AddDate(0, -3, 0), track.State{Exit: track.ExitOK})

	ids := func(f track.Filter, limit int) []string {
		t.Helper()
		found, err := s.FilteredTracks(ctx, f, now, limit)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, tr := range found {
			out = append(out, tr.ID)
		}
		return out
	}
	for _, tt := range []struct {
		f     track.Filter
		limit int
		want  []string
	}{
		{track.Filter{Started: track.Last30Days}, 500, []string{"active", "waiting", "done", "cleaned"}},
		{track.Filter{Started: track.Last30Days}, 2, []string{"active", "waiting"}},
		{track.Filter{Statuses: []string{"action_required", "closed"}}, 500, []string{"waiting"}},
		{track.Filter{Statuses: []string{"done"}}, 500, []string{"done", "cleaned"}},
		{track.Filter{Archived: true, Statuses: []string{"closed"}}, 500, []string{"archived"}},
		{track.Filter{Statuses: []string{"active"}}, 500, []string{"active"}},
		{track.Filter{Statuses: []string{"error"}}, 500, []string{"failed"}},
		{track.Filter{Statuses: []string{"exited", "action_required"}}, 500, []string{"waiting", "exited"}},
		{track.Filter{PRStatuses: []string{"none"}}, 1, []string{"active"}},
		{track.Filter{PRStatuses: []string{"open", "merged"}}, 500, []string{"waiting", "done"}},
		{track.Filter{Archived: true}, 500, []string{"archived"}},
		{track.Filter{Archived: true, PRStatuses: []string{"none"}}, 500, nil},
		{track.Filter{Started: track.Between, From: now.AddDate(0, 0, -10).Format(track.DateLayout), To: now.AddDate(0, 0, -3).Format(track.DateLayout)}, 500, []string{"done", "cleaned"}},
	} {
		if got := ids(tt.f, tt.limit); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("FilteredTracks(%s, %d) = %v, want %v", tt.f, tt.limit, got, tt.want)
		}
	}
}
