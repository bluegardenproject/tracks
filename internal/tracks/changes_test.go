package tracks

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/trackwin"
)

// received reports whether ch has a change waiting, and takes it.
func received(ch <-chan struct{}) bool {
	select {
	case _, ok := <-ch:
		return ok
	default:
		return false
	}
}

func TestChanges(t *testing.T) {
	c := &Changes{}
	a, cancelA := c.Subscribe()
	b, _ := c.Subscribe()
	if !received(a) || !received(b) {
		t.Fatal("a new subscription doesn't receive right away")
	}
	c.Notify()
	c.Notify()
	if !received(a) || received(a) {
		t.Error("two changes close together don't arrive as one")
	}
	cancelA()
	c.Notify()
	if received(a) {
		t.Error("a cancelled subscription still receives")
	}
	c.Close()
	if !received(b) {
		t.Error("b lost the change before Close")
	}
	if _, ok := <-b; ok {
		t.Error("Close doesn't close the subscriptions")
	}
	if late, _ := c.Subscribe(); received(late) {
		t.Error("a subscription after Close isn't closed")
	}
	c.Notify()

	var none *Changes
	none.Notify()
	none.Close()
	if ch, _ := none.Subscribe(); received(ch) {
		t.Error("a nil Changes' subscription isn't closed")
	}
}

func TestWatchedNotifiesOnWrites(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "tracks.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	changes := &Changes{}
	w := Watched(db, changes)
	ch, _ := changes.Subscribe()
	received(ch)
	now := time.UnixMilli(1_790_000_000_000)
	pr, _ := track.ParsePR("https://github.com/acme/web/pull/7")
	merged := pr
	merged.State = track.PRMerged

	writes := []struct {
		name    string
		write   func() error
		changes bool
	}{
		{"AddTrack", func() error {
			return w.AddTrack(ctx, track.Track{ID: "a", Kind: track.Work, Name: "a", Engine: "claude", CreatedAt: now,
				Repos: []track.Repo{{Name: "web", Path: "/src/web", Branch: "tracks/a"}}})
		}, true},
		{"SetState", func() error { return w.SetState(ctx, "a", track.State{Waiting: true}) }, true},
		{"Rename", func() error { return w.Rename(ctx, "a", "b") }, true},
		{"SetBranch", func() error { return w.SetBranch(ctx, "a", 0, "fix") }, true},
		{"SetSetupDone", func() error { return w.SetSetupDone(ctx, "a", "web", true) }, true},
		{"AddTrackError", func() error {
			return w.AddTrackError(ctx, "a", track.Failure{Kind: track.ServerError, Subject: "web/web", Code: 1})
		}, true},
		{"ClearTrackErrors", func() error { _, err := w.ClearTrackErrors(ctx, "a", "", ""); return err }, true},
		{"ClearTrackErrors", func() error { _, err := w.ClearTrackErrors(ctx, "a", "", ""); return err }, false},
		{"AddProxyPort", func() error { return w.AddProxyPort(ctx, 3000) }, true},
		{"AddProxyPort", func() error { return w.AddProxyPort(ctx, 3000) }, false},
		{"SetProxyInput", func() error { return w.SetProxyInput(ctx, 3000, store.ProxyInput{Track: "a"}) }, true},
		{"SetProxyInput", func() error { return w.SetProxyInput(ctx, 4000, store.ProxyInput{}) }, false},
		{"RemoveProxyPort", func() error { return w.RemoveProxyPort(ctx, 3000) }, true},
		{"RemoveProxyPort", func() error { return w.RemoveProxyPort(ctx, 3000) }, false},
		{"SetSetupDone", func() error { return w.SetSetupDone(ctx, "a", "api", true) }, false},
		{"SetCost", func() error { return w.SetCost(ctx, "a", 1.5) }, true},
		{"AddTrackRepo", func() error { return w.AddTrackRepo(ctx, "a", track.Repo{Name: "api", Path: "/src/api"}) }, true},
		{"AddTrackRepo", func() error { return w.AddTrackRepo(ctx, "gone", track.Repo{Name: "api"}) }, false},
		{"Promote", func() error { return w.Promote(ctx, track.Track{ID: "a", Kind: track.Work, Session: "s2"}) }, true},
		{"Promote", func() error { return w.Promote(ctx, track.Track{ID: "gone", Kind: track.Work}) }, false},
		{"SetFilter", func() error { return w.SetFilter(ctx, track.Filter{Archived: true}) }, true},
		{"AddPR", func() error { _, err := w.AddPR(ctx, "a", pr, now); return err }, true},
		{"AddPR", func() error { _, err := w.AddPR(ctx, "a", pr, now); return err }, false},
		{"SavePR", func() error { _, err := w.SavePR(ctx, "a", merged, now); return err }, true},
		{"SavePR", func() error { _, err := w.SavePR(ctx, "a", merged, now); return err }, false},
		{"Rename", func() error { return w.Rename(ctx, "gone", "b") }, false},
		{"SaveDraft", func() error { return w.SaveDraft(ctx, store.Draft{ID: "d", Request: "{}", FailedAt: now}) }, true},
		{"DeleteDraft", func() error { _, err := w.DeleteDraft(ctx, "d"); return err }, true},
		{"DeleteDraft", func() error { _, err := w.DeleteDraft(ctx, "d"); return err }, false},
		{"DeleteTrack", func() error { return w.DeleteTrack(ctx, "a") }, true},
		{"DeleteTrack", func() error { return w.DeleteTrack(ctx, "a") }, false},
	}
	tested := map[string]bool{}
	for _, c := range writes {
		tested[c.name] = true
		err := c.write()
		if c.changes && err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := received(ch); got != c.changes {
			t.Errorf("%s notified = %v; want %v", c.name, got, c.changes)
		}
	}

	reads := map[string]bool{"Repos": true, "Track": true, "OpenTracks": true, "EndedTracks": true,
		"EndedBefore": true, "InterruptedTracks": true, "FilteredTracks": true, "ProxyPorts": true, "Filter": true, "UnsettledPRs": true, "Drafts": true, "Draft": true,
		// A write no screen shows, so it needn't notify.
		"ClaimPorts": true}
	methods := reflect.TypeFor[Store]()
	for i := range methods.NumMethod() {
		if name := methods.Method(i).Name; !reads[name] && !tested[name] {
			t.Errorf("Store.%s is neither a read nor a write Watched is tested with", name)
		}
	}
}

func TestSweepNotifiesWhenWindowsChange(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	changes := &Changes{}
	f.svc.Changes = changes
	ch, _ := changes.Subscribe()
	received(ch)

	f.windows.windows = []trackwin.Info{{Number: 1, Window: "@1", Track: "x", Name: "one"}}
	if err := f.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if !received(ch) {
		t.Error("a new window isn't a change")
	}
	if err := f.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if received(ch) {
		t.Error("a Sweep that sees the same windows is a change")
	}
	f.windows.windows[0].Number = 2
	if err := f.svc.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	if !received(ch) {
		t.Error("a moved window isn't a change")
	}
}
