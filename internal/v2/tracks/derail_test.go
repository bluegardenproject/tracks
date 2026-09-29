package tracks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/store"
	"github.com/bluegardenproject/tracks/internal/v2/track"
	"github.com/bluegardenproject/tracks/internal/v2/workspace"
)

func TestDerail(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	work, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id := work.Track.ID
	if _, err := f.svc.Derail(ctx, id, true); err == nil || !strings.Contains(err.Error(), "End fix-it before") {
		t.Errorf("derailing an open track: %v", err)
	}
	hooks := filepath.Join(f.svc.HooksDir, id)
	if _, err := os.Stat(hooks); err != nil {
		t.Fatalf("the track has no hooks to delete: %v", err)
	}

	if _, err := f.svc.Lost(ctx, id); err == nil || !strings.Contains(err.Error(), "End fix-it before") {
		t.Errorf("checking an open track: %v", err)
	}
	if err := f.svc.End(ctx, id); err != nil {
		t.Fatal(err)
	}
	f.worktrees.lost = []workspace.Unsaved{{Repo: "api", Commits: 2}}
	if lost, err := f.svc.Lost(ctx, id); err != nil || len(lost) != 1 {
		t.Fatalf("Lost = %v, %v; want the work Derail would lose", lost, err)
	}
	if lost, err := f.svc.Derail(ctx, id, false); err != nil || len(lost) != 1 {
		t.Fatalf("Derail = %v, %v; want the work it would lose", lost, err)
	}
	if _, err := f.store.Track(ctx, id); err != nil || len(f.worktrees.derailed) != 0 {
		t.Fatalf("Derail deleted something with work to lose: %v, %v", err, f.worktrees.derailed)
	}
	if lost, err := f.svc.Derail(ctx, id, true); err != nil || len(lost) != 0 {
		t.Fatalf("forced Derail = %v, %v", lost, err)
	}
	if _, err := f.store.Track(ctx, id); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("the track is still saved: %v", err)
	}
	if !slices.Equal(f.worktrees.derailed, []string{id}) {
		t.Errorf("derailed worktrees %v", f.worktrees.derailed)
	}
	if _, err := os.Stat(hooks); !os.IsNotExist(err) {
		t.Errorf("the hooks are still there: %v", err)
	}
	if _, err := f.svc.Derail(ctx, id, true); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Errorf("derailing it again: %v", err)
	}
}

func TestDerailArchivedAndWithoutWorktrees(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	work, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := f.svc.Create(ctx, Request{Kind: track.Ask, Prompt: "Why"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{work.Track.ID, ask.Track.ID} {
		if err := f.svc.End(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.svc.Archive(ctx, work.Track.ID, true); err != nil {
		t.Fatal(err)
	}
	f.worktrees.lost = []workspace.Unsaved{{Repo: "api", Commits: 1}}
	if lost, err := f.svc.Lost(ctx, ask.Track.ID); err != nil || len(lost) != 0 {
		t.Errorf("checking a track without worktrees = %v, %v", lost, err)
	}
	if lost, err := f.svc.Derail(ctx, ask.Track.ID, false); err != nil || len(lost) != 0 {
		t.Errorf("derailing a track without worktrees = %v, %v", lost, err)
	}
	if lost, err := f.svc.Derail(ctx, work.Track.ID, false); err != nil || len(lost) != 1 {
		t.Errorf("an archived track's branch still counts: %v, %v", lost, err)
	}
	f.worktrees.lost = nil
	if _, err := f.svc.Derail(ctx, work.Track.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{work.Track.ID, ask.Track.ID} {
		if _, err := f.store.Track(ctx, id); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s is still saved: %v", id, err)
		}
	}
	if !slices.Equal(f.worktrees.derailed, []string{work.Track.ID}) {
		t.Errorf("derailed worktrees %v; want only the work track's", f.worktrees.derailed)
	}
}
