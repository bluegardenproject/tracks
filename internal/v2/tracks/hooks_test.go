package tracks

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/agents"
	"github.com/bluegardenproject/tracks/internal/v2/track"
)

func TestTracksGetTheirHooks(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	got, err := f.svc.Create(ctx, Request{Kind: track.Work, Repos: []string{"api"}, Prompt: "Fix it"}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	id := got.Track.ID
	dir := filepath.Join(f.svc.HooksDir, id)
	if want := filepath.Join(dir, "claude-settings.json"); f.engine.spec.Hooks != want {
		t.Fatalf("the agent starts with hooks %q, want %q", f.engine.spec.Hooks, want)
	}
	b, err := os.ReadFile(f.engine.spec.Hooks)
	if err != nil || !strings.Contains(string(b), "'/data/bin/tracks' hook --engine claude --track "+id) {
		t.Errorf("the settings don't run this track's hook: %s, %v", b, err)
	}

	if err := f.svc.End(ctx, id); err != nil {
		t.Fatal(err)
	}
	f.engine.spec = agents.Spec{}
	if _, err := f.svc.Resume(ctx, id, false, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if f.engine.spec.Hooks == "" {
		t.Error("a resumed track starts without its hooks")
	}

	if err := f.svc.End(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Clean(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Clean left the hooks: %v", err)
	}
}

func TestFailedCreateRemovesTheHooks(t *testing.T) {
	f := newFixture(t)
	f.engine.failCommand = true
	if _, err := f.svc.Create(context.Background(), Request{Kind: track.Ask, Prompt: "Why"}, func(string) {}); err == nil {
		t.Fatal("Create should fail")
	}
	if entries, _ := os.ReadDir(f.svc.HooksDir); len(entries) != 0 {
		t.Errorf("a failed Create left hooks: %v", entries)
	}
}
