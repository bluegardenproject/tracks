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

func TestShellCommandInjectsSocketDirAndBinDirOnPath(t *testing.T) {
	o := SpawnOptions{
		CLIBinary: "claude",
		TrackID:   "tid",
		SocketDir: "/sock/dir",
		BinDir:    "/opt/tracks/bin",
	}
	cmd := o.ShellCommand()
	if !strings.Contains(cmd, "TRACKS_SOCKET_DIR=") || !strings.Contains(cmd, "/sock/dir") {
		t.Errorf("expected TRACKS_SOCKET_DIR in command, got: %s", cmd)
	}
	if !strings.Contains(cmd, `PATH=`) || !strings.Contains(cmd, "/opt/tracks/bin") || !strings.Contains(cmd, `:"$PATH"`) {
		t.Errorf("expected BinDir prepended to PATH, got: %s", cmd)
	}
}

func TestShellCommandOmitsPathWhenNoBinDir(t *testing.T) {
	o := SpawnOptions{CLIBinary: "claude", TrackID: "tid", SocketDir: "/sock/dir"}
	cmd := o.ShellCommand()
	if strings.Contains(cmd, "PATH=") {
		t.Errorf("PATH should not be set when BinDir is empty, got: %s", cmd)
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

func TestBuildOptionsWorkRequiresRepos(t *testing.T) {
	cfg := config.Default()
	tr := baseTrack(state.KindWork)
	tr.Repos = nil
	if _, err := BuildOptions(cfg, tr, "/sock", ""); err == nil {
		t.Error("expected error when a work track has no repos")
	}
}

// A worktree-less ask may run with no repos (a general question). It
// should build cleanly, carry no --add-dir, and send the prompt
// verbatim — no read-only suffix (nothing local to protect) and none
// of the work-track framing.
func TestBuildOptionsAskAllowsNoRepos(t *testing.T) {
	cfg := config.Default()
	tr := baseTrack(state.KindAsk)
	tr.Repos = nil
	opts, err := BuildOptions(cfg, tr, "/sock", "")
	if err != nil {
		t.Fatalf("ask with no repos should build, got %v", err)
	}
	if len(opts.AddDirs) != 0 {
		t.Errorf("expected no --add-dir, got %v", opts.AddDirs)
	}
	if opts.PermissionMode != "plan" {
		t.Errorf("permission mode = %q, want plan", opts.PermissionMode)
	}
	if strings.Contains(opts.TaskPrompt, "read-only track") {
		t.Error("repo-less ask should not carry the read-only suffix")
	}
	if opts.TaskPrompt != "do the thing" {
		t.Errorf("repo-less ask prompt should be verbatim, got %q", opts.TaskPrompt)
	}
}

// Worktree-less tracks must not get the work-track suffix (review
// gate, PR marker, Jira sync) — that framing is wrong for a read-only
// question.
func TestBuildOptionsAskOmitsWorkSuffix(t *testing.T) {
	cfg := config.Default()
	opts, err := BuildOptions(cfg, baseTrack(state.KindAsk), "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(opts.TaskPrompt, "TRACKS_PR_URL") {
		t.Error("ask prompt should not carry the work-track suffix")
	}
}
