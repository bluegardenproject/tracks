package daemon

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// acquireLock places an exclusive flock on s.lockPath, preventing
// multiple daemons from racing into the same socket directory.
// Returns a clear error message when another daemon already holds
// the lock.
func (s *Server) acquireLock() error {
	f, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open lock %s: %w", s.lockPath, err)
	}
	// LOCK_NB returns immediately if the lock is held, rather than
	// blocking forever.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return fmt.Errorf("another tracks daemon is already running (lock %s held)", s.lockPath)
		}
		return fmt.Errorf("flock: %w", err)
	}
	s.lockFile = f
	return nil
}

// releaseLock unlocks and closes the flock file. Safe to call when
// the lock was never acquired.
func (s *Server) releaseLock() {
	if s.lockFile == nil {
		return
	}
	_ = syscall.Flock(int(s.lockFile.Fd()), syscall.LOCK_UN)
	_ = s.lockFile.Close()
	s.lockFile = nil
}
