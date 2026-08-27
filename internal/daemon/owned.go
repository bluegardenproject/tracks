package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bluegardenproject/tracks/internal/dlog"
)

// managedMarker is the frontmatter key every file tracks installs into
// the user's home directory carries. Its presence is what makes the
// file tracks' to replace.
//
// The `x-` prefix keeps it out of the way of the schemas Claude and
// Cursor actually read: both ignore unknown frontmatter keys, so the
// marker is inert to them and meaningful only to us.
const managedMarker = "x-tracks-managed:"

// markerScanLimit bounds how far into a file the marker is looked for.
// It belongs in the frontmatter block at the top; scanning the whole of
// a file a user may have grown to any size is pointless.
const markerScanLimit = 4096

// writeManagedFile installs one of tracks' own files, and refuses to
// touch anything else.
//
// tracks writes into ~/.claude and ~/.cursor — directories users
// populate themselves. Overwriting our own file on every daemon start
// is intended: it is how an upgrade reaches an existing install. What
// must never happen is clobbering a file with the same name that the
// user wrote, and the two are indistinguishable by path alone, which
// is why the marker exists.
//
// Reports whether the file was written.
func writeManagedFile(path string, content []byte) (bool, error) {
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// Nothing there — ours to create.
	case err != nil:
		return false, fmt.Errorf("read %s: %w", path, err)
	case !hasManagedMarker(existing):
		// No marker. Either it predates the marker — every install
		// before it has three unmarked files that tracks itself wrote —
		// or it belongs to the user. Adopt only if it is exactly what we
		// would have written, which is precisely the pre-marker case.
		if !isPreMarkerCopy(existing, content) {
			// Somebody else's file at a name we want. Leave it alone and
			// say so: silently skipping is how a user ends up wondering
			// why the review subagent never runs.
			dlog.Printf("not overwriting %s: no %s marker and the contents are not a previous tracks version, so it is not ours to replace", path, managedMarker)
			return false, nil
		}
		dlog.Printf("adopting %s: unmarked but identical to the shipped file, so it predates the ownership marker", path)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// hasManagedMarker reports whether content carries the ownership
// marker in its opening frontmatter.
func hasManagedMarker(content []byte) bool {
	head := content
	if len(head) > markerScanLimit {
		head = head[:markerScanLimit]
	}
	return strings.Contains(string(head), managedMarker)
}

// isPreMarkerCopy reports whether existing is what tracks would have
// written before the ownership marker existed — that is, want with the
// marker line taken out.
//
// Without this, the release that introduced the marker would orphan
// every file already installed: they carry no marker, so they would
// read as the user's and never be updated again. An exact match is the
// only signal that is safe here; anything looser starts guessing about
// files tracks did not write.
func isPreMarkerCopy(existing, want []byte) bool {
	return string(existing) == string(stripMarkerLine(want))
}

// stripMarkerLine removes the ownership-marker line from content.
func stripMarkerLine(content []byte) []byte {
	lines := strings.Split(string(content), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), managedMarker) {
			continue
		}
		out = append(out, l)
	}
	return []byte(strings.Join(out, "\n"))
}
