package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/config"
	"github.com/bluegardenproject/tracks/internal/state"
)

// The exact wrapper text, pinned. Two providers will generate this, and
// a difference between them is not a wording difference — it is a track
// that never reports finishing, or a helper that can't find its daemon.
// If this needs updating, the change is real and belongs in a commit
// message.
func TestCommandShape(t *testing.T) {
	got := Wrapper{
		TrackID:      "tid",
		SocketDir:    "/sock",
		BinDir:       "/opt/bin",
		SentinelPath: "/tmp/sentinel",
	}.Command("claude 'do the thing'")

	want := `TRACKS_ID='tid' TRACKS_SOCKET_DIR='/sock' PATH='/opt/bin':"$PATH" sh -c ` +
		`'claude '\''do the thing'\''` + "\n" + `touch '\''/tmp/sentinel'\''` + "\n" +
		`exec ${SHELL:-bash} -l'`
	if got != want {
		t.Errorf("wrapper drift:\n got: %s\nwant: %s", got, want)
	}
}

func TestCommandOmitsOptionalParts(t *testing.T) {
	got := Wrapper{TrackID: "tid", SocketDir: "/sock"}.Command("claude")
	if strings.Contains(got, "PATH=") {
		t.Errorf("PATH set with no BinDir: %s", got)
	}
	if strings.Contains(got, "touch") {
		t.Errorf("sentinel touched with no SentinelPath: %s", got)
	}
	if !strings.Contains(got, "exec ${SHELL:-bash} -l") {
		t.Errorf("pane would not fall back to a login shell: %s", got)
	}
}

// The env vars must reach the agent's own child processes — that is
// what the in-worktree helper scripts read, and it is why the design
// note "Cursor doesn't read CLAUDE.md so drop the env" was wrong.
func TestEnvReachesTheProcess(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	cmd := Wrapper{TrackID: "abc123", SocketDir: "/sock/dir"}.
		Command(`sh -c 'printf %s:%s "$TRACKS_ID" "$TRACKS_SOCKET_DIR"'`)
	// Drop the login-shell tail so the command terminates under test.
	cmd = strings.Replace(cmd, "\nexec ${SHELL:-bash} -l", "", 1)

	out, err := exec.Command("sh", "-c", cmd).Output()
	if err != nil {
		t.Fatalf("running the wrapped command: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "abc123:/sock/dir" {
		t.Errorf("env seen by the agent = %q, want abc123:/sock/dir", got)
	}
}

// The sentinel is how the supervisor learns the agent exited without
// watching the pid.
func TestSentinelIsTouchedAfterTheAgentExits(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	sentinel := filepath.Join(t.TempDir(), "done")
	cmd := Wrapper{TrackID: "t", SocketDir: "/s", SentinelPath: sentinel}.Command("true")
	cmd = strings.Replace(cmd, "\nexec ${SHELL:-bash} -l", "", 1)

	if err := exec.Command("sh", "-c", cmd).Run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Errorf("sentinel not created: %v", err)
	}
}

func TestDraftPRSuffixNamesReposOnlyWhenMixed(t *testing.T) {
	if got := DraftPRSuffix([]string{"a"}, 1); strings.Contains(got, "these repos") {
		t.Errorf("single-repo case named repos: %s", got)
	}
	got := DraftPRSuffix([]string{"a"}, 2)
	if !strings.Contains(got, "a.") || !strings.Contains(got, "these repos") {
		t.Errorf("mixed case should name the opted-in repos: %s", got)
	}
}

func TestDraftPRReposSelectsOnlyOptedIn(t *testing.T) {
	cfg := config.Config{Repos: []config.Repo{
		{Name: "yes", DraftPRs: true},
		{Name: "no"},
	}}
	got := DraftPRRepos(cfg, []state.TrackRepo{{Name: "yes"}, {Name: "no"}, {Name: "unknown"}})
	if len(got) != 1 || got[0] != "yes" {
		t.Errorf("DraftPRRepos = %v, want [yes]", got)
	}
}

// Line's whole reason to exist is the quoting guarantee, so it gets
// its own tests rather than only transitive coverage from providers.
func TestLineQuotesValuesAndNotFlags(t *testing.T) {
	got := string(NewLine("agent").
		Arg("it's a prompt").
		Flag("--force").
		Set("--model", "gpt-5.3-codex").
		Build())
	want := `'agent' 'it'\''s a prompt' --force --model 'gpt-5.3-codex'`
	if got != want {
		t.Errorf("Line built:\n got: %s\nwant: %s", got, want)
	}
}

// An empty value must omit the flag, not pass it empty: --model ”
// selects a model named "".
func TestSetIfOmitsEmptyValues(t *testing.T) {
	got := string(NewLine("agent").SetIf("--model", "").SetIf("--mode", "plan").Build())
	if strings.Contains(got, "--model") {
		t.Errorf("empty value produced a flag: %s", got)
	}
	if !strings.Contains(got, "--mode 'plan'") {
		t.Errorf("non-empty value was dropped: %s", got)
	}
}

// The binary itself is quoted — it comes from config and may contain a
// space in a path.
func TestLineQuotesTheBinary(t *testing.T) {
	got := string(NewLine("/Applications/My Tools/agent").Build())
	if !strings.Contains(got, `'/Applications/My Tools/agent'`) {
		t.Errorf("binary not quoted: %s", got)
	}
}
