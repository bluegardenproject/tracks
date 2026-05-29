package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func makeTrack(id string) Track {
	return Track{
		ID:         id,
		Branch:     "fix/example",
		Repos:      []TrackRepo{{Name: "ledger-live", Path: "/tmp/" + id + "/ledger-live"}},
		Status:     StatusRunning,
		LogPath:    "/tmp/" + id + ".jsonl",
		TaskPrompt: "do the thing",
	}
}

func TestFileStoreRoundtrip(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Put(makeTrack("a")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Put(makeTrack("b")); err != nil {
		t.Fatal(err)
	}
	// Re-open from disk and check that both tracks survived.
	fs2, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	all := fs2.All()
	if len(all) != 2 {
		t.Fatalf("got %d tracks, want 2", len(all))
	}
	if _, ok := fs2.Get("b"); !ok {
		t.Error("track b missing after reload")
	}
}

func TestFileStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Put(makeTrack("a")); err != nil {
		t.Fatal(err)
	}
	// No stray temp files left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := 0
	for _, e := range entries {
		if e.Name() == "state.json" {
			got++
			continue
		}
		t.Errorf("unexpected file in state dir: %s", e.Name())
	}
	if got != 1 {
		t.Errorf("state.json not found in %s", dir)
	}
}

func TestFileStoreDelete(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = fs.Put(makeTrack("a"))
	existed, err := fs.Delete("a")
	if err != nil || !existed {
		t.Fatalf("Delete: existed=%v err=%v", existed, err)
	}
	existed, err = fs.Delete("a")
	if err != nil || existed {
		t.Fatalf("Delete idempotent: existed=%v err=%v", existed, err)
	}
}

func TestFileStoreSortedByCreatedAt(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t1 := makeTrack("first")
	t1.CreatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := makeTrack("second")
	t2.CreatedAt = time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	// Insert in reverse order.
	_ = fs.Put(t2)
	_ = fs.Put(t1)
	all := fs.All()
	if all[0].ID != "first" || all[1].ID != "second" {
		t.Errorf("not sorted by CreatedAt: %v", []string{all[0].ID, all[1].ID})
	}
}

func TestFileStoreRejectsEmptyID(t *testing.T) {
	dir := t.TempDir()
	fs, err := OpenFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Put(Track{ID: ""}); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestFileStoreFutureSchemaRefused(t *testing.T) {
	dir := t.TempDir()
	bogus := map[string]any{
		"schema_version": CurrentSchemaVersion + 1,
		"tracks":         []any{},
	}
	data, _ := json.Marshal(bogus)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFileStore(dir); err == nil {
		t.Fatal("expected error opening future-schema state")
	}
}

func TestMemoryStoreImplementsStoreContract(t *testing.T) {
	var s Store = NewMemoryStore()
	_ = s.Put(makeTrack("x"))
	if got, ok := s.Get("x"); !ok || got.ID != "x" {
		t.Fatalf("Get(x): %+v ok=%v", got, ok)
	}
	if len(s.All()) != 1 {
		t.Fatalf("All: %d", len(s.All()))
	}
	if existed, err := s.Delete("x"); err != nil || !existed {
		t.Fatalf("Delete: existed=%v err=%v", existed, err)
	}
}

func TestStatusIsTerminal(t *testing.T) {
	cases := map[Status]bool{
		StatusPending: false,
		StatusRunning: false,
		StatusWaiting: false,
		StatusDone:    true,
		StatusErrored: true,
	}
	for s, want := range cases {
		if s.IsTerminal() != want {
			t.Errorf("%s.IsTerminal() = %v, want %v", s, !want, want)
		}
	}
}
