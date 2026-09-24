package tmux_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/tmux"
	"github.com/bluegardenproject/tracks/internal/v2/tmux/tmuxtest"
)

// The generated config must load cleanly into the installed tmux, and
// the session lifecycle must work on a server of our own.
func TestServerWithGeneratedConfig(t *testing.T) {
	socket := tmuxtest.Socket(t)
	v, err := tmux.InstalledVersion()
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	confPath := filepath.Join(dir, "tmux.conf")
	conf := tmux.Conf{
		Version:         v,
		DefaultTerminal: "screen-256color",
		OuterTerm:       "xterm-test",
		TrueColor:       true,
		OverrideFile:    filepath.Join(dir, "missing.conf"),
	}
	if err := conf.Write(confPath); err != nil {
		t.Fatal(err)
	}

	c := tmux.New(socket)
	if c.HasSession("tracks") {
		t.Fatal("fresh server already has a session")
	}
	if err := c.NewSession(confPath, "tracks", "Tracks", "sleep 30"); err != nil {
		t.Fatal(err)
	}
	if !c.HasSession("tracks") {
		t.Fatal("session missing after NewSession")
	}

	tmuxOut := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("tmux", append([]string{"-L", socket}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	// source-file fails on any config error, which new-session doesn't.
	tmuxOut("source-file", confPath)
	for option, want := range map[string]string{"escape-time": "0", "mouse": "on", "extended-keys": "on", "default-terminal": "screen-256color"} {
		if got := tmuxOut("show-options", "-gv", option); got != want {
			t.Errorf("option %s = %q, want %q", option, got, want)
		}
	}
	for _, want := range []string{"TRACKS_NEW_APP=1", "COLORTERM=truecolor"} {
		name, _, _ := strings.Cut(want, "=")
		if got := tmuxOut("show-environment", "-g", name); got != want {
			t.Errorf("environment = %q, want %s", got, want)
		}
	}
	if got := tmuxOut("show-options", "-gv", "terminal-features"); !strings.Contains(got, "xterm-test:RGB") {
		t.Errorf("terminal-features = %q, want 24-bit colour for xterm-test only", got)
	}
	if got := tmuxOut("display-message", "-p", "-t", "tracks:0", "#W"); got != "Tracks" {
		t.Errorf("window 0 = %q, want Tracks", got)
	}
	if err := c.SelectWindow("tracks", "0"); err != nil {
		t.Error(err)
	}

	if err := c.KillServer(); err != nil {
		t.Fatal(err)
	}
	if c.HasSession("tracks") {
		t.Error("session still there after KillServer")
	}
	if err := c.KillServer(); err != nil {
		t.Errorf("KillServer on a stopped server: %v", err)
	}
}
