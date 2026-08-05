package claude

import (
	"os"
	"path/filepath"
	"slices"
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

// A doc review is worktree-less but must NOT be forced into plan mode
// (the report save is a real write, and ExitPlanMode is the wrong
// framing for it). It gets the doc contract instead of the ask/plan
// read-only suffix, and the document's directory must be granted via
// --add-dir or Claude can't open the file at all.
func TestBuildOptionsDocReview(t *testing.T) {
	cfg := config.Default()
	cfg.Claude.PermissionMode = "acceptEdits"
	dir := t.TempDir()
	doc := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(doc, []byte("# spec\n"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	tr := baseTrack(state.KindDoc)
	tr.DocPath = doc

	opts, err := BuildOptions(cfg, tr, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.PermissionMode != "default" {
		t.Errorf("doc permission mode = %q, want acceptEdits clamped to default", opts.PermissionMode)
	}
	if !slices.Contains(opts.AddDirs, dir) {
		t.Errorf("AddDirs = %v, want it to contain the document's dir %q", opts.AddDirs, dir)
	}
	// The document's directory wins over an attached primary checkout.
	// (Only a narrowing: an in-repo document puts cwd in the repo anyway.)
	if opts.CWD != dir {
		t.Errorf("CWD = %q, want the document's dir %q even with repos attached", opts.CWD, dir)
	}
	if !strings.Contains(opts.TaskPrompt, doc) {
		t.Error("doc prompt should name the document under review")
	}
	if !strings.Contains(opts.TaskPrompt, "tracks-docs-reviewer") {
		t.Error("doc prompt should point at the docs-reviewer subagent")
	}
	if !strings.Contains(opts.TaskPrompt, ".review.md") {
		t.Error("doc prompt should carry the save-the-report protocol")
	}
	if strings.Contains(opts.TaskPrompt, "TRACKS_PR_URL") {
		t.Error("doc prompt should not carry the work-track suffix")
	}
	if strings.Contains(opts.TaskPrompt, "read-only track") {
		t.Error("doc prompt should carry its own contract, not the ask/plan suffix")
	}
}

// A doc review needs no repos: a deck that makes no claims about any
// code is a valid track. The pane then opens in the document's own
// directory rather than the user's home.
func TestBuildOptionsDocReviewAllowsNoRepos(t *testing.T) {
	cfg := config.Default()
	dir := t.TempDir()
	doc := filepath.Join(dir, "deck.pdf")
	if err := os.WriteFile(doc, []byte("%PDF-1.4\n"), 0o644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	tr := baseTrack(state.KindDoc)
	tr.Repos = nil
	tr.DocPath = doc

	opts, err := BuildOptions(cfg, tr, "/sock", "")
	if err != nil {
		t.Fatalf("doc review with no repos should build, got %v", err)
	}
	if opts.CWD != dir {
		t.Errorf("CWD = %q, want the document's dir %q", opts.CWD, dir)
	}
	if len(opts.AddDirs) != 1 || opts.AddDirs[0] != dir {
		t.Errorf("AddDirs = %v, want just the document's dir", opts.AddDirs)
	}
}

// The doc write contract is prose, and the track holds --add-dir grants
// on the user's primary checkouts, so the modes that skip prompting are
// clamped to one that asks. A stricter configured mode is respected.
func TestBuildOptionsDocReviewClampsPermissiveModes(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(doc, []byte("# spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := baseTrack(state.KindDoc)
	tr.DocPath = doc

	for configured, want := range map[string]string{
		"acceptEdits":       "default",
		"bypassPermissions": "default",
		"auto":              "default",
		"default":           "default",
		"plan":              "plan",
	} {
		cfg := config.Default()
		cfg.Claude.PermissionMode = configured
		opts, err := BuildOptions(cfg, tr, "/sock", "")
		if err != nil {
			t.Fatalf("%s: %v", configured, err)
		}
		if opts.PermissionMode != want {
			t.Errorf("configured %q gave doc permission mode %q, want %q", configured, opts.PermissionMode, want)
		}
	}
}

// Promotion turns a doc track into a work track but leaves DocPath on
// the record as provenance. The work session must not inherit the
// document's directory — as a cwd or as an --add-dir grant — or it ends
// up running the commit/push workflow from inside the primary checkout
// instead of its new worktree.
func TestBuildOptionsPromotedDocTrackIgnoresDocPath(t *testing.T) {
	cfg := config.Default()
	dir := t.TempDir()
	doc := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(doc, []byte("# spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := baseTrack(state.KindWork) // promoted: kind flipped, DocPath kept
	tr.DocPath = doc

	opts, err := BuildOptions(cfg, tr, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(opts.AddDirs, dir) {
		t.Errorf("AddDirs = %v, want no grant on the document's dir", opts.AddDirs)
	}
	if opts.CWD != "/tmp/demo" {
		t.Errorf("CWD = %q, want the worktree", opts.CWD)
	}
	if !strings.Contains(opts.TaskPrompt, "TRACKS_PR_URL") {
		t.Error("a promoted track should carry the work-track suffix")
	}
}

// Resuming a doc track restores its --add-dir grants (primary checkouts
// plus the document's directory) but not the prose contract, which only
// lives in the transcript — so the clamp has to be applied there too.
func TestBuildResumeOptionsDocReviewClampsMode(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(doc, []byte("# spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := baseTrack(state.KindDoc)
	tr.DocPath = doc
	tr.SessionID = "sess-1"

	cfg := config.Default()
	cfg.Claude.PermissionMode = "bypassPermissions"
	opts, err := BuildResumeOptions(cfg, tr, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.PermissionMode != "default" {
		t.Errorf("resumed doc permission mode = %q, want bypassPermissions clamped to default", opts.PermissionMode)
	}
	if !slices.Contains(opts.AddDirs, dir) {
		t.Errorf("AddDirs = %v, want the document's dir still granted on resume", opts.AddDirs)
	}

	// A resumed work track keeps whatever the user configured.
	work := baseTrack(state.KindWork)
	work.SessionID = "sess-2"
	opts, err = BuildResumeOptions(cfg, work, "/sock", "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.PermissionMode != "bypassPermissions" {
		t.Errorf("resumed work permission mode = %q, want the configured mode", opts.PermissionMode)
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
