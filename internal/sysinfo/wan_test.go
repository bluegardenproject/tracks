package sysinfo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestWANCachesAndFallsBack(t *testing.T) {
	answer, calls := "85.14.3.9", 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if answer == "" {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintln(w, answer)
	}))
	defer srv.Close()

	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	w := WAN{URL: srv.URL, CacheFile: filepath.Join(t.TempDir(), "wan.json"), MaxAge: 10 * time.Minute,
		Timeout: time.Second, Now: func() time.Time { return now }}
	ctx := context.Background()

	if got := w.Address(ctx); got != "85.14.3.9" {
		t.Fatalf("first lookup = %q", got)
	}
	answer = "85.14.3.10"
	now = now.Add(5 * time.Minute)
	if got := w.Address(ctx); got != "85.14.3.9" || calls != 1 {
		t.Errorf("fresh cache: got %q after %d calls, want the cached address and no new call", got, calls)
	}
	now = now.Add(10 * time.Minute)
	if got := w.Address(ctx); got != "85.14.3.10" {
		t.Errorf("stale cache: got %q, want a new lookup", got)
	}
	answer = ""
	now = now.Add(time.Hour)
	if got := w.Address(ctx); got != "85.14.3.10" {
		t.Errorf("failed lookup: got %q, want the last known address", got)
	}
}
