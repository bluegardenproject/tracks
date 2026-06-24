package claude

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

func baseTrack(kind state.Kind) state.Track {
	return state.Track{
		ID:         "20260101-000000-abcdef",
		Kind:       kind,
		TaskPrompt: "do the thing",
		Repos:      []state.TrackRepo{{Name: "demo", Path: "/tmp/demo"}},
	}
}

func TestBuildOptionsWorkUsesConfiguredMode(t *testing.T) {
	cfg := config.Default()
	cfg.Claude.PermissionMode = "acceptEdits"
	opts, err := BuildOptions(cfg, baseTrack(state.KindWork), "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.PermissionMode != "acceptEdits" {
		t.Errorf("work permission mode = %q, want acceptEdits", opts.PermissionMode)
	}
	if strings.Contains(opts.TaskPrompt, "read-only track") {
		t.Error("work prompt should not carry the read-only suffix")
	}
}

func TestBuildOptionsAskAndPlanForcePlanMode(t *testing.T) {
	cfg := config.Default()
	cfg.Claude.PermissionMode = "bypassPermissions" // should be overridden
	for _, kind := range []state.Kind{state.KindAsk, state.KindPlan} {
		opts, err := BuildOptions(cfg, baseTrack(kind), "/sock", "")
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if opts.PermissionMode != "plan" {
			t.Errorf("%s permission mode = %q, want plan", kind, opts.PermissionMode)
		}
		if !strings.Contains(opts.TaskPrompt, "read-only track") {
			t.Errorf("%s prompt missing read-only suffix", kind)
		}
	}
}

func TestBuildOptionsRequiresRepos(t *testing.T) {
	cfg := config.Default()
	tr := baseTrack(state.KindAsk)
	tr.Repos = nil
	if _, err := BuildOptions(cfg, tr, "/sock", ""); err == nil {
		t.Error("expected error when track has no repos")
	}
}
