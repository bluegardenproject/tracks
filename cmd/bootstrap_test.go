package cmd

import (
	"os"
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
