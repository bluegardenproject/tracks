package claude

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var safeSessionID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Locate returns a session's transcript files.
//
// Primary: look for `<sessionID>.jsonl` directly inside each project
// dir under `projects/` — exact, and free of any dependency on how
// Claude derives a project directory name from the cwd. Fallback (e.g.
// if a Claude build doesn't honor --session-id in interactive mode):
// derive the project dir from the worktree cwd and take every `.jsonl`
// transcript in it. Because each track has a unique worktree path, that
// directory holds only this track's sessions.
func Locate(sessionID, cwd string) []string {
	base, err := projectsDir()
	if err != nil {
		return nil
	}
	if sessionID != "" && sessionID != "." && sessionID != ".." && safeSessionID.MatchString(sessionID) {
		if entries, err := os.ReadDir(base); err == nil {
			var matches []string
			target := sessionID + ".jsonl"
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				p := filepath.Join(base, e.Name(), target)
				if info, err := os.Lstat(p); err == nil && info.Mode().IsRegular() {
					matches = append(matches, p)
				}
			}
			if len(matches) > 0 {
				return matches
			}
		}
	}
	if cwd != "" {
		dir := filepath.Join(base, sanitizeCWD(cwd))
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() {
			return nil
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var matches []string
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			p := filepath.Join(dir, e.Name())
			if info, err := os.Lstat(p); err == nil && info.Mode().IsRegular() {
				matches = append(matches, p)
			}
		}
		return matches
	}
	return nil
}

// projectsDir is where Claude Code stores per-project transcripts.
// Honors CLAUDE_CONFIG_DIR (which relocates the whole ~/.claude tree).
func projectsDir() (string, error) {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "projects"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// cwdSanitizer turns an absolute path into Claude's project-dir name:
// every '/' and '.' becomes '-' (verified empirically, e.g.
// /Users/x/.local/state/... → -Users-x--local-state-...).
var cwdSanitizer = strings.NewReplacer("/", "-", ".", "-")

func sanitizeCWD(cwd string) string { return cwdSanitizer.Replace(cwd) }

// modelDateSuffix matches the release-date tail some model ids carry,
// in either separator style and with Bedrock's trailing version:
// claude-haiku-4-5-20251001, claude-opus-4@20250514 (Vertex),
// anthropic.claude-opus-4-20250514-v1:0 (Bedrock).
var modelDateSuffix = regexp.MustCompile(`[-@]\d{8}(-v\d+:\d+)?$`)

// ShortModel renders a model id for a narrow column: the vendor prefix
// and any release-date suffix come off, so `claude-haiku-4-5-20251001`
// reads as `haiku-4-5` and `claude-opus-5` as `opus-5`. Gateway-hosted
// ids shorten to the same thing as their direct equivalents. An
// unrecognised id is returned trimmed but otherwise untouched.
func ShortModel(id string) string {
	s := strings.TrimSpace(id)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, "anthropic.") // Bedrock
	s = strings.TrimPrefix(s, "claude-")
	return modelDateSuffix.ReplaceAllString(s, "")
}
