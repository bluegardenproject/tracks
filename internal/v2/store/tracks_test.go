package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestTracksRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	web, err := s.AddRepo(ctx, Repo{Name: "web", Path: "/src/web", BaseBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	api, err := s.AddRepo(ctx, Repo{Name: "api", Path: "/src/api", BaseBranch: "develop"})
	if err != nil {
		t.Fatal(err)
	}
	created := time.UnixMilli(time.Now().UnixMilli())
	work := track.Track{
		ID: "20260928-151500-a1b2c3", Kind: track.Work, Name: "rate-bug", Title: "Rate bug", Engine: "claude", Model: "opus",
		Session: "0b6f…", Prompt: "Fix the rate bug", Terminal: true, Opinion: true, ClaimCheck: true, CreatedAt: created,
		Repos: []track.Repo{
			{RepoID: web.ID, Name: "web", Path: "/src/web", Worktree: "/wt/web", Branch: "tracks/a1b2c3", Base: "main"},
			{RepoID: api.ID, Name: "api", Path: "/src/api", Worktree: "/wt/api", Branch: "tracks/a1b2c3", Base: "develop"},
		},
	}
	doc := track.Track{
		ID: "20260928-151600-d4e5f6", Kind: track.Doc, Name: "deck", Engine: "cursor", Session: "chat-1",
		Prompt: "Review it", Document: "/docs/deck.pdf", Candor: 7, ClaimCheck: true, CreatedAt: created.Add(time.Second),
	}
	for _, tr := range []track.Track{work, doc} {
		if err := s.AddTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []track.Track{work, doc} {
		got, err := s.Track(ctx, want.ID)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Track(%s) =\n%+v, %v\nwant\n%+v", want.ID, got, err, want)
		}
	}

	if err := s.DeleteRepo(ctx, api.ID); err != nil {
		t.Fatal(err)
	}
	got, err := s.Track(ctx, work.ID)
	if err != nil || got.Repos[1].RepoID != 0 || got.Repos[1].Name != "api" {
		t.Errorf("after removing api the track has %+v, %v; want its copy without the link", got.Repos, err)
	}
	if _, err := s.Track(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown track: %v, want ErrNotFound", err)
	}
}

func TestOpenAndClosedTracks(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	now := time.Now()
	for i, id := range []string{"b", "a", "c"} {
		tr := track.Track{ID: id, Kind: track.Ask, Name: id, Engine: "claude", CreatedAt: now.Add(time.Duration(i) * time.Second)}
		if err := s.AddTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	closedAt := now.Add(time.Minute)
	if err := s.SetState(ctx, "a", track.State{ClosedAt: closedAt}); err != nil {
		t.Fatal(err)
	}
	listed, err := s.OpenTracks(ctx)
	if err != nil || len(listed) != 2 || listed[0].ID != "b" || listed[1].ID != "c" {
		t.Fatalf("OpenTracks = %+v, %v; want b and c, oldest first", listed, err)
	}
	a, _ := s.Track(ctx, "a")
	if a.Open() || a.ClosedAt.UnixMilli() != closedAt.UnixMilli() {
		t.Errorf("a closed at %v, want %v", a.ClosedAt, closedAt)
	}
}

func TestStateAndName(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	now := time.Now()
	for i, id := range []string{"a", "b", "c", "d"} {
		tr := track.Track{ID: id, Kind: track.Work, Name: id, Engine: "claude", CreatedAt: now.Add(time.Duration(i) * time.Second)}
		if err := s.AddTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	for i, id := range []string{"c", "a", "b"} {
		if err := s.SetState(ctx, id, track.State{ClosedAt: now.Add(time.Duration(i+1) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	ended, err := s.EndedTracks(ctx, 2)
	if err != nil || len(ended) != 2 || ended[0].ID != "b" || ended[1].ID != "a" {
		t.Fatalf("EndedTracks(2) = %+v, %v; want b then a, the last closed first", ended, err)
	}

	cleaned := track.State{ClosedAt: now, CleanedAt: now.Add(time.Hour)}
	if err := s.SetState(ctx, "b", cleaned); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.Track(ctx, "b"); b.CleanedAt.UnixMilli() != cleaned.CleanedAt.UnixMilli() || b.Status() != track.Done {
		t.Errorf("b = %+v, want done, cleaned at %v", b.State, cleaned.CleanedAt)
	}
	if err := s.SetState(ctx, "d", track.State{Waiting: true}); err != nil {
		t.Fatal(err)
	}
	if d, _ := s.Track(ctx, "d"); !d.Waiting || d.Status() != track.ActionRequired {
		t.Errorf("d = %+v, want waiting", d.State)
	}
	if err := errors.Join(s.SetState(ctx, "b", track.State{}), s.Rename(ctx, "b", "b-2"), s.SetCost(ctx, "b", 3.45)); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.Track(ctx, "b"); !b.Open() || b.Cleaned() || b.Name != "b-2" || b.Cost != 3.45 {
		t.Errorf("reopened b = %+v; want open, not cleaned, named b-2, $3.45", b)
	}
	if open, _ := s.OpenTracks(ctx); len(open) != 2 || open[0].ID != "b" || open[1].ID != "d" {
		t.Errorf("OpenTracks after reopening b = %+v", open)
	}
	for name, err := range map[string]error{
		"rename": s.Rename(ctx, "nope", "x"), "set the state of": s.SetState(ctx, "nope", track.State{}),
		"set the cost of": s.SetCost(ctx, "nope", 1), "add a repo to": s.AddTrackRepo(ctx, "nope", track.Repo{Name: "web"}),
	} {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s an unknown track: %v, want ErrNotFound", name, err)
		}
	}
}

func TestArchivedTracks(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	now := time.UnixMilli(1_790_000_000_000)
	for i, id := range []string{"a", "b", "c"} {
		closed := now.Add(-time.Duration(10-i) * 24 * time.Hour)
		tr := track.Track{ID: id, Kind: track.Ask, Name: id, Engine: "claude", CreatedAt: closed, State: track.State{ClosedAt: closed}}
		if err := s.AddTrack(ctx, tr); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetState(ctx, "b", track.State{ClosedAt: now.Add(-9 * 24 * time.Hour), ArchivedAt: now}); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.Track(ctx, "b"); !b.ArchivedAt.Equal(now) || !b.Archived() {
		t.Errorf("b = %+v, want archived at %v", b.State, now)
	}
	if ended, err := s.EndedTracks(ctx, 10); err != nil || len(ended) != 2 || ended[0].ID != "c" || ended[1].ID != "a" {
		t.Errorf("EndedTracks = %+v, %v; want c and a, not the archived b", ended, err)
	}
	if old, err := s.EndedBefore(ctx, now.Add(-8*24*time.Hour-time.Minute)); err != nil || len(old) != 1 || old[0].ID != "a" {
		t.Errorf("EndedBefore = %+v, %v; want a alone: c ended since, b is archived", old, err)
	}
}

func TestDatabaseIsPrivate(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "tracks.db")
	s := open(t, path)
	if err := s.AddTrack(ctx, track.Track{ID: "x", Kind: track.Ask, Engine: "claude", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]os.FileMode{dir: 0o700, path: 0o600, path + "-wal": 0o600, path + "-shm": 0o600} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s has mode %o, want %o", filepath.Base(p), got, want)
		}
	}
}

func TestAddTrackRepo(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	tr := track.Track{ID: "a", Kind: track.Work, Name: "a", Engine: "claude", CreatedAt: time.Now(),
		Repos: []track.Repo{{Name: "web", Path: "/src/web", Worktree: "/wt/a/web", Branch: "tracks/a", Base: "main"}}}
	if err := s.AddTrack(ctx, tr); err != nil {
		t.Fatal(err)
	}
	docs := track.Repo{Name: "docs", Path: "/src/docs", Worktree: "/wt/a/docs", Branch: "tracks/a", Base: "develop"}
	if err := s.AddTrackRepo(ctx, "a", docs); err != nil {
		t.Fatal(err)
	}
	got, err := s.Track(ctx, "a")
	if err != nil || len(got.Repos) != 2 || got.Repos[0].Name != "web" || got.Repos[1] != docs {
		t.Fatalf("repos %+v, %v; want web, then docs", got.Repos, err)
	}
	if err := s.SetBranch(ctx, "a", 1, "fix/docs"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Track(ctx, "a"); got.Repos[1].Branch != "fix/docs" || got.Repos[0].Branch != "tracks/a" {
		t.Errorf("the added repo isn't at position 1: %+v", got.Repos)
	}
}
