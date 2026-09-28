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
		ID: "20260928-151500-a1b2c3", Kind: track.Work, Name: "rate-bug", Engine: "claude", Model: "opus",
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
	if err := s.CloseTrack(ctx, "a", closedAt); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseTrack(ctx, "a", closedAt.Add(time.Hour)); err != nil {
		t.Errorf("closing twice: %v", err)
	}
	if err := s.CloseTrack(ctx, "nope", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("closing an unknown track: %v, want ErrNotFound", err)
	}
	listed, err := s.OpenTracks(ctx)
	if err != nil || len(listed) != 2 || listed[0].ID != "b" || listed[1].ID != "c" {
		t.Fatalf("OpenTracks = %+v, %v; want b and c, oldest first", listed, err)
	}
	a, _ := s.Track(ctx, "a")
	if a.Open() || a.ClosedAt.UnixMilli() != closedAt.UnixMilli() {
		t.Errorf("a closed at %v, want %v, kept on a second close", a.ClosedAt, closedAt)
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
