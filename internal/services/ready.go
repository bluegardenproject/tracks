package services

import (
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
	"time"
)

// Probe describes how to tell a service is ready. The zero value (both
// fields empty) means "ready as soon as it starts".
type Probe struct {
	// Port, when set, is satisfied once something accepts a TCP
	// connection on 127.0.0.1:<Port>. Already resolved to a number.
	Port string
	// LogRegex, when set, is satisfied once the service log matches this
	// RE2 pattern.
	LogRegex string
}

// IsZero reports whether the probe declares no readiness condition.
func (p Probe) IsZero() bool { return p.Port == "" && p.LogRegex == "" }

// DefaultReadyTimeout bounds how long WaitReady polls before giving up.
const DefaultReadyTimeout = 60 * time.Second

// WaitReady blocks until the probe is satisfied, ctx is cancelled, or
// timeout elapses. A zero probe returns immediately. logPath is the
// service's log file, read for the log_regex probe.
func WaitReady(ctx context.Context, probe Probe, logPath string, timeout time.Duration) error {
	if probe.IsZero() {
		return nil
	}
	var re *regexp.Regexp
	if probe.LogRegex != "" {
		var err error
		re, err = regexp.Compile(probe.LogRegex)
		if err != nil {
			return fmt.Errorf("bad log_regex %q: %w", probe.LogRegex, err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if probe.Port != "" && portOpen(probe.Port) {
			return nil
		}
		if re != nil && logMatches(logPath, re) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("not ready within %s: %w", timeout, ctx.Err())
		case <-ticker.C:
		}
	}
}

// portOpen reports whether a TCP listener accepts a connection on the
// loopback address at port.
func portOpen(port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// logMatches reports whether the file at path matches re. A missing or
// unreadable file is simply not-yet-matched.
func logMatches(path string, re *regexp.Regexp) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return re.Match(b)
}
