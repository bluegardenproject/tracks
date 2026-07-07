package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// valueAfter returns the argv element following the first occurrence of
// flag, or "" if not present.
func valueAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// TestNewWindowArgsTargetSessionExplicitly guards the regression where a
// bare `-t <session>` target let tmux match a WINDOW of that name in
// whichever session was active (one of the user's other sessions),
// landing the track window in the wrong session or failing with
// "index N in use". The target must be session-qualified ("<session>:").
func TestNewWindowArgsTargetSessionExplicitly(t *testing.T) {
	cases := map[string][]string{
		"NewWindow":                newWindowArgs("tracks", "my-track", "claude", "/repo", true),
		"NewWindowReturningPaneID": newWindowPaneArgs("tracks", "my-track", "claude", "/repo"),
	}
	for name, args := range cases {
		target := valueAfter(args, "-t")
		if target != "tracks:" {
			t.Errorf("%s: -t target = %q, want %q (bare session name leaks into other sessions)", name, target, "tracks:")
		}
		// It must never request an explicit index — tmux must pick the
		// next free slot within the session.
		if strings.HasPrefix(target, "tracks:") && strings.TrimPrefix(target, "tracks:") != "" {
			t.Errorf("%s: target %q pins an explicit window index; must let tmux choose", name, target)
		}
	}
}

// TestNewWindowLandsInTargetSessionDespiteOtherSessions reproduces the
// real bug end-to-end: another session is active and owns a window named
// like our session. The fixed code must create the window in our session
// and never touch the other one. Skipped when tmux isn't installed.
func TestNewWindowLandsInTargetSessionDespiteOtherSessions(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	uniq := fmt.Sprintf("trktest-%d-%d", os.Getpid(), time.Now().UnixNano())
	trackSess := uniq + "-tracks"
	otherSess := uniq + "-other"

	mustTmux := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
			t.Fatalf("tmux %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}

	mustTmux("new-session", "-d", "-s", trackSess, "-n", "Dashboard")
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", trackSess).Run() })
	mustTmux("new-session", "-d", "-s", otherSess, "-n", "w0")
	t.Cleanup(func() { _ = exec.Command("tmux", "kill-session", "-t", otherSess).Run() })

	// The other session owns a window whose name equals our session name,
	// and is the most-recently-active session — the exact trap that made a
	// bare `-t <session>` resolve into the wrong session.
	mustTmux("new-window", "-t", otherSess+":1", "-n", trackSess)
	mustTmux("new-window", "-t", otherSess+":2", "-n", "filler")
	mustTmux("select-window", "-t", otherSess+":1")

	if _, err := (Client{}).NewWindowReturningPaneID(trackSess, "claude-x", "", ""); err != nil {
		t.Fatalf("NewWindowReturningPaneID: %v", err)
	}

	got, err := (Client{}).HasWindow(trackSess, "claude-x")
	if err != nil || !got {
		t.Errorf("expected window in %q; HasWindow=%v err=%v", trackSess, got, err)
	}
	if leaked, _ := (Client{}).HasWindow(otherSess, "claude-x"); leaked {
		t.Errorf("window leaked into the other session %q", otherSess)
	}
}
