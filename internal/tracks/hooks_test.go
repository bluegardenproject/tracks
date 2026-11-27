package tracks

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/agents"
	"github.com/bluegardenproject/tracks/internal/track"
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
	if want := filepath.Join(dir, "claude-mcp.json"); f.engine.spec.MCPConfig != want {
		t.Errorf("the agent starts with the MCP config %q, want %q", f.engine.spec.MCPConfig, want)
	}
	first := f.engine.spec.Token
	if got, err := f.svc.AgentTrack(ctx, first); first == "" || err != nil || got != id {
		t.Errorf("the agent's token %q is %q's, %v; want %s's", first, got, err, id)
	}

	if err := f.svc.End(ctx, id); err != nil {
		t.Fatal(err)
	}
	f.engine.spec = agents.Spec{}
	// A review left by a build that kept it after the track ended.
	run := filepath.Join(f.svc.legacyReviewDir(id), "run")
	if err := os.MkdirAll(filepath.Dir(run), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(run, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Resume(ctx, id, false, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if f.engine.spec.Hooks == "" {
		t.Error("a resumed track starts without its hooks")
	}
	if got, err := f.svc.AgentTrack(ctx, f.engine.spec.Token); f.engine.spec.Token == first || err != nil || got != id {
		t.Errorf("the resumed agent's token is %q's, %v; want a new one, %s's", got, err, id)
	}
	var p Problem
	if _, err := f.svc.AgentTrack(ctx, first); !errors.As(err, &p) {
		t.Errorf("the last session's token: %v, want a problem", err)
	}
	if exists(run) {
		t.Error("a resumed track kept the review of its last run")
	}

	if err := f.svc.End(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Archive(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Archive left the hooks: %v", err)
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
