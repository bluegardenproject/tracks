package footer

import (
	"fmt"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/sysinfo"
	"github.com/bluegardenproject/tracks/internal/v2/theme"
)

// System is the system data shown for s. Values that couldn't be read
// are left out.
func System(s sysinfo.Snapshot, t theme.Theme, dark bool) string {
	p := palette{t, dark}
	var items []string
	add := func(label, value string) {
		items = append(items, p.fg(theme.FooterMuted)+label+" "+p.fg(theme.FooterText)+value)
	}
	if s.LAN != "" {
		add("LAN", s.LAN)
	}
	if s.WAN != "" {
		add("WAN", s.WAN)
	}
	if s.CPUKnown {
		add("CPU", fmt.Sprintf("%.0f%%", s.CPU))
	}
	if s.MemTotal > 0 {
		add("MEM", fmt.Sprintf("%s / %s GB", gigabytes(s.MemUsed), gigabytes(s.MemTotal)))
	}
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, p.fg(theme.FooterFaint)+"  ·  ") + "#[default]"
}

func gigabytes(b uint64) string {
	return fmt.Sprintf("%.1f", float64(b)/(1<<30))
}
