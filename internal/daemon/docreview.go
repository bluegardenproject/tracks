package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// unreadableDocExts are document formats Claude cannot open directly:
// they're zip/binary containers, not text or images, so `Read` returns
// nothing useful. Reviewing them would need a conversion step
// (LibreOffice et al) that `tracks` deliberately doesn't own — the user
// exports to PDF instead, which is one manual step and zero fragility.
//
// The check is a deny-list rather than an allow-list on purpose:
// anything else (.md, .txt, .csv, .json, .svg, source files, …) is
// text Claude can read, and an allow-list would need extending forever.
var unreadableDocExts = map[string]string{
	".pptx":    "PowerPoint",
	".ppt":     "PowerPoint",
	".key":     "Keynote",
	".odp":     "OpenDocument presentation",
	".docx":    "Word",
	".doc":     "Word",
	".odt":     "OpenDocument text",
	".xlsx":    "Excel",
	".xls":     "Excel",
	".numbers": "Numbers",
	".pages":   "Pages",
}

// ResolveDocPath turns a user-supplied document path into an absolute,
// existing path suitable for a KindDoc track. Accepts `~`-relative and
// relative paths, and either a single file or a directory of files
// (e.g. a deck exported as one image per slide).
//
// Exported so the new-track form can validate the field as it's typed
// and reject a `.pptx` before a track is ever created — the daemon
// re-runs the same check, since a draft can be relaunched later.
func ResolveDocPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("empty document path")
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expand %q: %w", p, err)
		}
		p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", p, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no such file or directory: %s", abs)
		}
		return "", fmt.Errorf("stat %s: %w", abs, err)
	}
	if !info.IsDir() {
		if format, bad := unreadableDocExts[strings.ToLower(filepath.Ext(abs))]; bad {
			return "", fmt.Errorf("%s files can't be read directly (%s) — export it to PDF and point the track at that", format, filepath.Base(abs))
		}
	}
	return abs, nil
}
