// Package tmuxtest gives tests a throwaway tmux server.
package tmuxtest

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/tmux"
)

// Socket returns a unique socket name for t. When the test ends it
// kills that server and removes the socket file, which tmux leaves
// behind. It skips the test when tmux isn't installed.
func Socket(t testing.TB) string {
	t.Helper()
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	socket := tmux.TestSocketPrefix + hex.EncodeToString(b)
	t.Cleanup(func() {
		_ = tmux.New(socket).KillServer()
		_ = os.Remove(socketPath(socket))
	})
	return socket
}

// socketPath is where tmux puts a `-L` socket.
func socketPath(socket string) string {
	dir := os.Getenv("TMUX_TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	return filepath.Join(dir, fmt.Sprintf("tmux-%d", os.Getuid()), socket)
}
