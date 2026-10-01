package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestDrafts(t *testing.T) {
	ctx := context.Background()
	s := open(t, filepath.Join(t.TempDir(), "tracks.db"))
	now := time.UnixMilli(1_790_000_000_000)

	if err := s.SaveDraft(ctx, Draft{ID: "b", Request: `{"kind":"ask"}`, Error: "no engine", FailedAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDraft(ctx, Draft{ID: "a", Request: `{"kind":"work"}`, Error: "no repo", FailedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDraft(ctx, Draft{ID: "a", Request: `{"kind":"plan"}`, Error: "name taken", FailedAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	drafts, err := s.Drafts(ctx)
	if err != nil || len(drafts) != 2 {
		t.Fatalf("Drafts = %+v, %v; want two", drafts, err)
	}
	a := drafts[0]
	if a.ID != "a" || a.Request != `{"kind":"plan"}` || a.Error != "name taken" || !a.FailedAt.Equal(now) {
		t.Errorf("the updated draft = %+v; want the new request and error, first failure kept, listed first", a)
	}
	if d, err := s.Draft(ctx, "b"); err != nil || d.Error != "no engine" {
		t.Errorf("Draft(b) = %+v, %v", d, err)
	}

	if deleted, err := s.DeleteDraft(ctx, "a"); err != nil || !deleted {
		t.Fatalf("DeleteDraft(a) = %v, %v", deleted, err)
	}
	if deleted, err := s.DeleteDraft(ctx, "a"); err != nil || deleted {
		t.Errorf("deleting it again = %v, %v; want nothing deleted", deleted, err)
	}
	if _, err := s.Draft(ctx, "a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Draft(a) after deleting = %v, want ErrNotFound", err)
	}
}
