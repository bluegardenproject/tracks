package cursor

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/agent"
	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

func baseCfg() config.Config {
	cfg := config.Default()
	cfg.Cursor.Binary = "agent"
	return cfg
}

func baseTrack(kind state.Kind) state.Track {
	return state.Track{
		ID:         "20260101-000000-abcdef",
		Kind:       kind,
		Provider:   state.ProviderCursor,
		TaskPrompt: "do the thing",
		SessionID:  "08ab06bf-8ec1-495f-90c9-b7523131b824",
		Repos:      []state.TrackRepo{{Name: "demo", Path: "/tmp/demo"}},
	}
}

// --resume carries the chat id from the very first run: CreateChat
// makes an empty chat, so there is no separate "start" form. Getting
// this wrong means every track silently begins a new conversation.
func TestFirstRunResumesTheCreatedChat(t *testing.T) {
	opts, err := BuildOptions(baseCfg(), baseTrack(state.KindWork), "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	cmd := opts.ShellCommand()
	if !strings.Contains(cmd, "--resume") || !strings.Contains(cmd, "08ab06bf") {
		t.Errorf("first run does not resume the created chat: %s", cmd)
	}
	if strings.Contains(cmd, "--session-id") {
		t.Errorf("--session-id is Claude's flag, not Cursor's: %s", cmd)
	}
}

// The permission vocabulary differs from Claude's; passing Claude's
// flags to Cursor would fail at spawn.
func TestWorkTracksForceAndNeverUsePermissionMode(t *testing.T) {
	opts, _ := BuildOptions(baseCfg(), baseTrack(state.KindWork), "/sock", "")
	cmd := opts.ShellCommand()
	if !strings.Contains(cmd, "--force") {
		t.Errorf("work track should run with --force: %s", cmd)
	}
	if strings.Contains(cmd, "--permission-mode") {
		t.Errorf("--permission-mode is Claude's flag: %s", cmd)
	}
	// "--mode" is a prefix of "--model"; assert the field, not the string.
	if opts.Mode != "" {
		t.Errorf("work track should not be in a read-only mode, got %q", opts.Mode)
	}
}

// ask and plan map onto Cursor's own read-only modes, and must not be
// forced.
func TestReadOnlyKindsMapToCursorModes(t *testing.T) {
	for kind, want := range map[state.Kind]string{
		state.KindAsk:  "ask",
		state.KindPlan: "plan",
	} {
		opts, err := BuildOptions(baseCfg(), baseTrack(kind), "/sock", "")
		if err != nil {
			t.Fatal(err)
		}
		// The inner command is quoted into the outer sh -c, so the flag
		// and its value are asserted separately rather than as one
		// literal.
		cmd := opts.ShellCommand()
		if !strings.Contains(cmd, "--mode ") || !strings.Contains(cmd, want) {
			t.Errorf("%s track: want --mode %s, got: %s", kind, want, cmd)
		}
		if opts.Mode != want {
			t.Errorf("%s track: Mode = %q, want %q", kind, opts.Mode, want)
		}
		if strings.Contains(cmd, "--force") {
			t.Errorf("%s track must not be forced: %s", kind, cmd)
		}
		if !strings.Contains(opts.TaskPrompt, "read-only track") {
			t.Errorf("%s track lost the read-only contract from its prompt", kind)
		}
	}
}

// The first repo is the workspace; the rest are extra roots. Cursor
// defaults --workspace to cwd, so naming it explicitly is what keeps
// the pane's directory and the agent's workspace from drifting.
func TestReposBecomeWorkspaceAndAddDirs(t *testing.T) {
	tr := baseTrack(state.KindWork)
	tr.Repos = []state.TrackRepo{
		{Name: "a", Path: "/tmp/a"},
		{Name: "b", Path: "/tmp/b"},
		{Name: "c", Path: "/tmp/c"},
	}
	opts, err := BuildOptions(baseCfg(), tr, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Workspace != "/tmp/a" || opts.CWD != "/tmp/a" {
		t.Errorf("workspace/cwd = %q/%q, want /tmp/a", opts.Workspace, opts.CWD)
	}
	cmd := opts.ShellCommand()
	if strings.Count(cmd, "--add-dir") != 2 {
		t.Errorf("want 2 --add-dir for the non-primary repos, got: %s", cmd)
	}
}

// Same rule as the Claude path, verified against the real CLI: a
// resumed chat restores the model it was last using, so forcing
// --model would undo a switch made in the pane.
func TestResumePassesNoModelAndNoPrompt(t *testing.T) {
	cfg := baseCfg()
	cfg.Cursor.Model = "gpt-5.3-codex"
	opts, err := BuildResumeOptions(cfg, baseTrack(state.KindWork), "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Model != "" {
		t.Errorf("resume carries Model = %q, want empty", opts.Model)
	}
	cmd := opts.ShellCommand()
	if strings.Contains(cmd, "--model") {
		t.Errorf("--model must not be passed on resume: %s", cmd)
	}
	if strings.Contains(cmd, "do the thing") {
		t.Errorf("resume must not re-send the task prompt: %s", cmd)
	}
	if !strings.Contains(cmd, "--resume") {
		t.Errorf("resume lost its chat id: %s", cmd)
	}
}

func TestResumeRequiresAChatID(t *testing.T) {
	tr := baseTrack(state.KindWork)
	tr.SessionID = ""
	if _, err := BuildResumeOptions(baseCfg(), tr, "/sock", ""); err == nil {
		t.Error("resume without a chat id should fail rather than start a new conversation")
	}
}

// The track's own pick beats the configured default, and the per-kind
// default applies when it has none — same contract as the Claude path.
func TestModelResolution(t *testing.T) {
	cfg := baseCfg()
	cfg.Cursor.Model = "auto"
	cfg.Cursor.ModelByKind = map[string]string{"work": "gpt-5.3-codex"}

	opts, _ := BuildOptions(cfg, baseTrack(state.KindWork), "/sock", "")
	if opts.Model != "gpt-5.3-codex" {
		t.Errorf("Model = %q, want the per-kind default", opts.Model)
	}

	tr := baseTrack(state.KindWork)
	tr.RequestedModel = "composer-2.5"
	opts, _ = BuildOptions(cfg, tr, "/sock", "")
	if opts.Model != "composer-2.5" {
		t.Errorf("Model = %q, want the track's own pick", opts.Model)
	}
}

// The env and sentinel come from agent.Wrapper. This pins that the
// Cursor path actually calls it — the guard the reviewer noted was
// convention until a second provider existed.
func TestCursorUsesTheSharedWrapper(t *testing.T) {
	opts, _ := BuildOptions(baseCfg(), baseTrack(state.KindWork), "/sock", "/tmp/sentinel")
	opts.BinDir = "/opt/bin"
	cmd := opts.ShellCommand()
	// The env prefix is outside the quoted inner command, so it appears
	// literally; the sentinel is inside it and comes through escaped.
	for _, want := range []string{
		"TRACKS_ID=", "TRACKS_SOCKET_DIR=", `PATH='/opt/bin':"$PATH"`,
		"touch ", "/tmp/sentinel", "exec ${SHELL:-bash} -l", "sh -c ",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("missing %q — the shared wrapper was bypassed:\n%s", want, cmd)
		}
	}
}

// A work track must carry the review instruction. It is weaker than
// Claude's subagent gate by necessity, but it must not be absent.
func TestWorkPromptKeepsAReviewGate(t *testing.T) {
	opts, _ := BuildOptions(baseCfg(), baseTrack(state.KindWork), "/sock", "")
	// The gate must send the agent to `tracks review`, not have it
	// review its own diff: the command is what gives the reviewer a
	// fresh context, and what refuses a review from inside a review.
	if !strings.Contains(opts.TaskPrompt, "tracks review") {
		t.Error("work prompt does not invoke `tracks review` — the command would ship unreachable")
	}
	if strings.Contains(opts.TaskPrompt, "Review your own work") {
		t.Error("work prompt still asks the agent to review its own diff")
	}
	if !strings.Contains(opts.TaskPrompt, "TRACKS_PR_URL=") {
		t.Error("work prompt lost the PR marker contract the dashboard needs")
	}
	if strings.Contains(opts.TaskPrompt, "subagent_type") {
		t.Error("Cursor prompt references Claude's subagent mechanism")
	}
}

func TestWorktreelessTrackNeedsNoRepos(t *testing.T) {
	tr := baseTrack(state.KindAsk)
	tr.Repos = nil
	opts, err := BuildOptions(baseCfg(), tr, "/sock", "")
	if err != nil {
		t.Fatalf("a repo-less ask track should build: %v", err)
	}
	if opts.CWD == "" {
		t.Error("no cwd — tmux needs a valid directory to open the pane in")
	}
}

func TestWorkTrackRequiresRepos(t *testing.T) {
	tr := baseTrack(state.KindWork)
	tr.Repos = nil
	if _, err := BuildOptions(baseCfg(), tr, "/sock", ""); err == nil {
		t.Error("a work track with no repos should fail")
	}
}

// The doc branch is the one with the workspace juggling and the safety
// contract, and it had neither a test nor the contract in the first
// version of this package.
func TestDocTrackCarriesTheWriteContract(t *testing.T) {
	tr := baseTrack(state.KindDoc)
	tr.Doc = &state.DocSpec{Path: "/tmp/spec.md"}
	opts, err := BuildOptions(baseCfg(), tr, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		agent.DocWriteContract, agent.DocSaveFlow, agent.DocResponseStyle,
	} {
		if !strings.Contains(opts.TaskPrompt, want) {
			t.Errorf("doc prompt is missing a shared contract block:\n%s", want[:60])
		}
	}
	if opts.Force {
		t.Error("a doc track must not run with --force; the write should prompt")
	}
}

// A section the user switched off must be stated as OFF, not omitted:
// an absent line reads as unspecified and invites the model to run it.
func TestDocBriefStatesBothSwitchesEitherWay(t *testing.T) {
	for _, tc := range []struct{ skipOpinion, skipClaims bool }{
		{false, false}, {true, false}, {false, true}, {true, true},
	} {
		tr := baseTrack(state.KindDoc)
		tr.Doc = &state.DocSpec{Path: "/tmp/spec.md", SkipOpinion: tc.skipOpinion, SkipClaimCheck: tc.skipClaims}
		opts, err := BuildOptions(baseCfg(), tr, "/sock", "")
		if err != nil {
			t.Fatal(err)
		}
		wantOpinion := "Opinion section: ON"
		if tc.skipOpinion {
			wantOpinion = "Opinion section: OFF"
		}
		wantClaims := "Claim check section: ON"
		if tc.skipClaims {
			wantClaims = "Claim check section: OFF"
		}
		if !strings.Contains(opts.TaskPrompt, wantOpinion) || !strings.Contains(opts.TaskPrompt, wantClaims) {
			t.Errorf("skipOpinion=%v skipClaims=%v: brief did not state both switches", tc.skipOpinion, tc.skipClaims)
		}
	}
}

// The candor level the creation form collects has to reach the model;
// on the first version of this package it was silently discarded.
func TestReviewTrackCarriesCandor(t *testing.T) {
	tr := baseTrack(state.KindReview)
	tr.Review = &state.ReviewSpec{Candor: 3}
	opts, err := BuildOptions(baseCfg(), tr, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(opts.TaskPrompt, "Review candor: 3/10") {
		t.Errorf("review prompt lost the candor level:\n%s", opts.TaskPrompt)
	}
}

// A track with no chat id must fail rather than launch a conversation
// tracks has no id for and can never resume.
func TestBuildOptionsRequiresAChatID(t *testing.T) {
	tr := baseTrack(state.KindWork)
	tr.SessionID = ""
	if _, err := BuildOptions(baseCfg(), tr, "/sock", ""); err == nil {
		t.Error("a missing chat id should fail rather than start an unresumable track")
	}
}

// Resume blanks fields by denylist, so a new prompt-derived field can
// ride along. Check every kind, not just work.
func TestResumeCarriesNoPromptForAnyKind(t *testing.T) {
	for _, kind := range []state.Kind{state.KindWork, state.KindReview, state.KindAsk, state.KindPlan, state.KindDoc} {
		tr := baseTrack(kind)
		if kind == state.KindDoc {
			tr.Doc = &state.DocSpec{Path: "/tmp/spec.md"}
		}
		opts, err := BuildResumeOptions(baseCfg(), tr, "/sock", "")
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if opts.TaskPrompt != "" {
			t.Errorf("%s resume carries a prompt: %q", kind, opts.TaskPrompt[:40])
		}
		if opts.Model != "" {
			t.Errorf("%s resume carries a model: %q", kind, opts.Model)
		}
	}
}

// The gate has to produce something checkable, or there is no way to
// tell a real review from an assertion of compliance.
func TestReviewGateIsCheckable(t *testing.T) {
	opts, _ := BuildOptions(baseCfg(), baseTrack(state.KindWork), "/sock", "")
	for _, want := range []string{
		"REVIEW OUTCOME: pass",        // greppable verdict, same convention as Claude's
		"Do not push with unresolved", // the negative constraint
		"tracks review",               // an observable action, not a disposition
		"do NOT invoke `agent`",       // the agent must not spawn the reviewer itself
	} {
		if !strings.Contains(opts.TaskPrompt, want) {
			t.Errorf("review gate is missing %q", want)
		}
	}
}

// tracks' dev-server capability reaches Cursor identically; without
// the instruction the agent runs pnpm dev in its own pane and blocks.
func TestWorkPromptCarriesTheDevServerContract(t *testing.T) {
	opts, _ := BuildOptions(baseCfg(), baseTrack(state.KindWork), "/sock", "")
	if !strings.Contains(opts.TaskPrompt, agent.DevServerContract) {
		t.Error("work prompt lost the shared dev-server contract")
	}
}
