package agents

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ManagedMarker is the frontmatter key of every file tracks installs
// into the user's home. Its presence is what makes a file tracks' to
// replace; the agents ignore frontmatter keys they don't know.
const ManagedMarker = "x-tracks-managed:"

// markerScanLimit bounds how far into a file the marker is looked for:
// it belongs in the frontmatter at the top.
const markerScanLimit = 4096

// WriteManaged writes content to path unless a file without the marker
// is there, and reports whether path now holds content.
func WriteManaged(path string, content []byte) (bool, error) {
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return false, fmt.Errorf("read %s: %w", path, err)
	case bytes.Equal(existing, content):
		return true, nil
	case !bytes.Contains(existing[:min(len(existing), markerScanLimit)], []byte(ManagedMarker)):
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, err
	}
	return true, nil
}
