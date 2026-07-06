package claude

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

const draftMarker = "`gh pr create --draft`"

func cfgWithRepo(name string, draft bool) config.Config {
	cfg := config.Default()
	cfg.Repos = []config.Repo{{Name: name, Path: "/tmp/" + name, Base: "main", DraftPRs: draft}}
	return cfg
}

func TestBuildOptionsInjectsDraftInstructionWhenRepoOptsIn(t *testing.T) {
	cfg := cfgWithRepo("demo", true)
	opts, err := BuildOptions(cfg, baseTrack(state.KindWork), "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(opts.TaskPrompt, draftMarker) {
		t.Errorf("work prompt should instruct draft PRs when the repo opts in; got:\n%s", opts.TaskPrompt)
	}
}

func TestBuildOptionsNoDraftInstructionByDefault(t *testing.T) {
	cfg := cfgWithRepo("demo", false)
	opts, err := BuildOptions(cfg, baseTrack(state.KindWork), "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(opts.TaskPrompt, draftMarker) {
		t.Error("work prompt should not mention draft PRs when the repo hasn't opted in")
	}
}

func TestBuildOptionsAskTrackNeverDrafts(t *testing.T) {
	// Worktree-less tracks don't get the work suffix at all, so the draft
	// instruction must not appear even if the repo opts in.
	cfg := cfgWithRepo("demo", true)
	opts, err := BuildOptions(cfg, baseTrack(state.KindAsk), "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(opts.TaskPrompt, draftMarker) {
		t.Error("ask track should never carry the draft-PR instruction")
	}
}
