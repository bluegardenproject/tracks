package footer

import (
	"regexp"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/sysinfo"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

var styles = regexp.MustCompile(`#\[[^\]]*\]`)

func TestSystem(t *testing.T) {
	tests := []struct {
		name string
		s    sysinfo.Snapshot
		want string
	}{
		{"everything", sysinfo.Snapshot{LAN: "192.168.1.20", WAN: "85.14.3.9", CPU: 12.4, CPUKnown: true, MemUsed: 9 << 30, MemTotal: 16 << 30},
			"LAN 192.168.1.20  ·  WAN 85.14.3.9  ·  CPU 12%  ·  MEM 9.0 / 16.0 GB"},
		{"offline", sysinfo.Snapshot{CPU: 0, CPUKnown: true, MemUsed: 1 << 29, MemTotal: 8 << 30},
			"CPU 0%  ·  MEM 0.5 / 8.0 GB"},
		{"nothing", sysinfo.Snapshot{}, ""},
	}
	for _, tt := range tests {
		got := styles.ReplaceAllString(System(tt.s, theme.Default(), true), "")
		if got != tt.want {
			t.Errorf("%s:\n got %q\nwant %q", tt.name, got, tt.want)
		}
	}
}
