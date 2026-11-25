package agents

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ManagedMarker is the frontmatter key of every file tracks installs
// into the user's home. Its presence is what makes a file tracks' to
// replace; the agents ignore frontmatter keys they don't know.
const ManagedMarker = "x-tracks-managed:"

// markerScanLimit bounds how far into a file the marker is looked for:
// it belongs in the frontmatter at the top.
const markerScanLimit = 4096

// WriteManaged writes content to path unless a file without the marker
// (or a symlink / non-regular entry) is there, and reports whether path
// now holds content.
func WriteManaged(path string, content []byte) (bool, error) {
	limit := max(markerScanLimit, len(content)+1)
	existing, perm, exists, regular, err := readManagedPrefix(path, limit)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	mode := fs.FileMode(0o644)
	if exists {
		if !regular {
			return false, nil
		}
		if bytes.Equal(existing, content) {
			return true, nil
		}
		if !hasManagedMarker(existing[:min(len(existing), markerScanLimit)]) {
			return false, nil
		}
		mode = perm
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	if err := writeAtomic(dir, path, content, mode); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveManaged removes path when it's a regular file tracks installed,
// one with the marker in its frontmatter, and leaves anything else as it is.
func RemoveManaged(path string) error {
	existing, _, exists, regular, err := readManagedPrefix(path, markerScanLimit)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if !exists || !regular || !hasManagedMarker(existing) {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// readManagedPrefix inspects path without following symlinks and reads
// at most limit bytes when it is a regular file.
func readManagedPrefix(path string, limit int) (data []byte, perm fs.FileMode, exists, regular bool, err error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, false, false, nil
	}
	if err != nil {
		return nil, 0, false, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, true, false, nil
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, 0, false, false, nil
	}
	if errors.Is(err, syscall.ELOOP) {
		return nil, 0, true, false, nil
	}
	if err != nil {
		return nil, 0, true, false, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, 0, true, false, err
	}
	if !st.Mode().IsRegular() {
		return nil, 0, true, false, nil
	}
	data, err = io.ReadAll(io.LimitReader(f, int64(limit)))
	return data, st.Mode().Perm(), true, true, err
}

// hasManagedMarker reports whether head begins with YAML frontmatter
// containing a top-level active x-tracks-managed key.
func hasManagedMarker(head []byte) bool {
	s := string(head)
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		rest, ok = strings.CutPrefix(s, "---\r\n")
	}
	if !ok {
		return false
	}
	for _, raw := range strings.Split(rest, "\n") {
		raw = strings.TrimSuffix(raw, "\r")
		trimmed := strings.TrimSpace(raw)
		if trimmed == "---" || trimmed == "..." {
			break
		}
		// Require column 0 so indented multiline scalars or nested maps
		// cannot trigger a false match.
		val, ok := strings.CutPrefix(raw, ManagedMarker)
		if !ok {
			continue
		}
		val = yamlScalar(val)
		if val != "" && !strings.EqualFold(val, "false") && val != "0" {
			return true
		}
		return false
	}
	return false
}

// yamlScalar extracts the scalar value from a single-line YAML value,
// stripping quotes and trailing comments outside quotes.
func yamlScalar(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 0 && (v[0] == '"' || v[0] == '\'') {
		q := v[0]
		if end := strings.IndexByte(v[1:], q); end >= 0 {
			return strings.TrimSpace(v[1 : 1+end])
		}
	}
	if i := strings.IndexByte(v, '#'); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

func writeAtomic(dir, path string, content []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(dir, ".tracks-managed-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(name)
		}
	}()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
