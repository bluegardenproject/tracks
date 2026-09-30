package claude

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// managedMarker is the frontmatter key of every file tracks installs
// into the user's home. Its presence is what makes a file tracks' to
// replace; Claude ignores frontmatter keys it doesn't know.
const managedMarker = "x-tracks-managed:"

// markerScanLimit bounds how far into a file the marker is looked for:
// it belongs in the frontmatter at the top.
const markerScanLimit = 4096

// InstallHelpers writes what the prompts rely on into home's .claude:
// the reviewer subagents, and the add-repo skill. A file there without
// the marker is the user's and stays untouched; its path is returned
// in skipped.
func InstallHelpers(home string) (skipped []string, err error) {
	dir := filepath.Join(home, ".claude")
	for _, f := range []struct{ name, content string }{
		{"agents/tracks-v2-reviewer.md", reviewerAgent},
		{"agents/tracks-v2-docs-reviewer.md", docsReviewerAgent},
		{"skills/tracks-v2-add-repo/SKILL.md", addRepoSkill},
	} {
		path := filepath.Join(dir, filepath.FromSlash(f.name))
		ok, err := writeManaged(path, []byte(f.content))
		if err != nil {
			return skipped, err
		}
		if !ok {
			skipped = append(skipped, path)
		}
	}
	return skipped, nil
}

// writeManaged writes content to path unless a file without the marker
// is there, and reports whether path now holds content.
func writeManaged(path string, content []byte) (bool, error) {
	existing, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return false, fmt.Errorf("read %s: %w", path, err)
	case bytes.Equal(existing, content):
		return true, nil
	case !bytes.Contains(existing[:min(len(existing), markerScanLimit)], []byte(managedMarker)):
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
