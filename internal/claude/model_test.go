package claude

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

func TestShellCommandPassesTheModel(t *testing.T) {
	o := SpawnOptions{CLIBinary: "claude", TrackID: "tid", Model: "claude-opus-4-8"}
	cmd := o.ShellCommand()
	// The inner command is shell-quoted into the outer sh -c, so the
	// flag and its value are asserted separately rather than as one
	// literal.
	if !strings.Contains(cmd, "--model") || !strings.Contains(cmd, "claude-opus-4-8") {
		t.Errorf("expected --model with the pinned id, got: %s", cmd)
	}
}

func TestShellCommandOmitsTheModelWhenUnset(t *testing.T) {
	o := SpawnOptions{CLIBinary: "claude", TrackID: "tid"}
	if cmd := o.ShellCommand(); strings.Contains(cmd, "--model") {
		t.Errorf("--model should be absent when no model is set, got: %s", cmd)
	}
}

// A resumed session restores the model it was last using, so forcing
// --model would silently undo a `/model` switch the user made in the
// pane. Verified against the CLI: resuming without the flag came back
// on the pinned model, not the default.
func TestResumeNeverPassesTheModel(t *testing.T) {
	o := SpawnOptions{
		CLIBinary: "claude",
		TrackID:   "tid",
		SessionID: "sess-1",
		Resume:    true,
		Model:     "claude-opus-4-8",
	}
	cmd := o.ShellCommand()
	if strings.Contains(cmd, "--model") {
		t.Errorf("--model must not be passed on a resume, got: %s", cmd)
	}
	if !strings.Contains(cmd, "--resume") {
		t.Errorf("expected --resume, got: %s", cmd)
	}
}

// BuildResumeOptions must not populate Model at all, so the guard above
// is belt-and-braces rather than the only thing standing between a
// resume and a forced model.
func TestBuildResumeOptionsLeavesTheModelEmpty(t *testing.T) {
	cfg := config.Default()
	cfg.Claude.Model = "claude-opus-4-8"
	tr := baseTrack(state.KindWork)
	tr.SessionID = "sess-1"
	tr.RequestedModel = "claude-sonnet-5"

	opts, err := BuildResumeOptions(cfg, tr, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Model != "" {
		t.Errorf("resume options carry Model = %q, want empty", opts.Model)
	}
}

func TestBuildOptionsPrefersTheTracksOwnModel(t *testing.T) {
	cfg := config.Default()
	cfg.Claude.Model = "claude-opus-5"
	tr := baseTrack(state.KindWork)
	tr.RequestedModel = "claude-opus-4-8"

	opts, err := BuildOptions(cfg, tr, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Model != "claude-opus-4-8" {
		t.Errorf("Model = %q, want the track's own pick to beat the config default", opts.Model)
	}
}

// A track created before the picker existed carries no model of its
// own, and must still pick up the configured default for its kind.
func TestBuildOptionsFallsBackToThePerKindDefault(t *testing.T) {
	cfg := config.Default()
	cfg.Claude.Model = "claude-opus-5"
	cfg.Claude.ModelByKind = map[string]string{"doc": "haiku"}

	work, err := BuildOptions(cfg, baseTrack(state.KindWork), "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if work.Model != "claude-opus-5" {
		t.Errorf("work Model = %q, want the global default", work.Model)
	}

	doc := baseTrack(state.KindDoc)
	doc.Doc = &state.DocSpec{Path: "/tmp/doc.md"}
	got, err := BuildOptions(cfg, doc, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "haiku" {
		t.Errorf("doc Model = %q, want the per-kind override", got.Model)
	}
}

// No model configured anywhere means no flag — Claude's own default
// applies, exactly as before this existed.
func TestBuildOptionsPassesNoModelWhenNoneConfigured(t *testing.T) {
	opts, err := BuildOptions(config.Default(), baseTrack(state.KindWork), "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Model != "" {
		t.Errorf("Model = %q, want empty when nothing is configured", opts.Model)
	}
	if strings.Contains(opts.ShellCommand(), "--model") {
		t.Error("no --model flag should reach the command line")
	}
}
