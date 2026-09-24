package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/daemon"
)

func TestDaemonStaleReason(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	fi, err := os.Stat(self)
	if err != nil {
		t.Fatalf("stat self: %v", err)
	}
	mod := fi.ModTime().UnixNano()

	// Ensure a deterministic Version for the test regardless of ldflags.
	orig := Version
	Version = "test-version"
	t.Cleanup(func() { Version = orig })

	tests := []struct {
		name  string
		ping  daemon.PingResult
		stale bool
	}{
		{
			name:  "version mismatch is stale",
			ping:  daemon.PingResult{Version: "other", ExePath: self, ExeModUnixNano: mod},
			stale: true,
		},
		{
			name:  "same version and current binary is fresh",
			ping:  daemon.PingResult{Version: "test-version", ExePath: self, ExeModUnixNano: mod},
			stale: false,
		},
		{
			name:  "same version but older binary mtime is stale",
			ping:  daemon.PingResult{Version: "test-version", ExePath: self, ExeModUnixNano: mod - 1},
			stale: true,
		},
		{
			name:  "different exe path is not judged by mtime",
			ping:  daemon.PingResult{Version: "test-version", ExePath: "/some/other/tracks", ExeModUnixNano: mod - 1},
			stale: false,
		},
		{
			name:  "missing mtime falls back to version only",
			ping:  daemon.PingResult{Version: "test-version", ExePath: self, ExeModUnixNano: 0},
			stale: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := daemonStaleReason(tt.ping) != ""
			if got != tt.stale {
				t.Errorf("daemonStaleReason(%+v) stale=%v, want %v (reason=%q)",
					tt.ping, got, tt.stale, daemonStaleReason(tt.ping))
			}
		})
	}
}

func TestForeignDaemonError(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "tracks")
	if err := os.WriteFile(installed, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "tracks-link")
	if err := os.Symlink(installed, link); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		daemon  string
		self    string
		refused bool
	}{
		{"same binary restarts", installed, installed, false},
		{"symlink to the same binary restarts", installed, link, false},
		{"unknown daemon path restarts", "", installed, false},
		{"other binary is left alone", "/Users/me/.tracks/tracks", installed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := foreignDaemonError(daemon.PingResult{Version: "1.2.0", ExePath: tt.daemon}, tt.self, "tracks")
			if (err != nil) != tt.refused {
				t.Errorf("foreignDaemonError(daemon=%q, self=%q) = %v, want refused=%v", tt.daemon, tt.self, err, tt.refused)
			}
		})
	}
}
