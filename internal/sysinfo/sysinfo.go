// Package sysinfo reads what Tracks shows about this machine: its LAN
// and WAN addresses, and CPU and memory use.
package sysinfo

import (
	"context"
	"net"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
)

// Snapshot is the machine at one moment. Fields that couldn't be read
// are zero ("" or 0), and MemTotal 0 means memory is unknown.
type Snapshot struct {
	LAN, WAN          string
	CPU               float64 // percent of all cores
	CPUKnown          bool
	MemUsed, MemTotal uint64 // bytes
}

// LAN returns the address this machine uses for outgoing traffic, ""
// when offline. Nothing is sent: connecting a UDP socket only picks the
// route.
func LAN() string {
	conn, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return ""
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return ""
}

// CPU measures CPU use over sample.
func CPU(ctx context.Context, sample time.Duration) (float64, error) {
	p, err := cpu.PercentWithContext(ctx, sample, false)
	if err != nil || len(p) == 0 {
		return 0, err
	}
	return p[0], nil
}

// Memory returns the memory in use and in total, in bytes.
func Memory(ctx context.Context) (used, total uint64, err error) {
	m, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return 0, 0, err
	}
	return m.Used, m.Total, nil
}
