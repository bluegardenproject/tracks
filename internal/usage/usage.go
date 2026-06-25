// Package usage reads Claude Code's own session transcripts to total
// the token usage and USD cost of a track.
//
// Why the transcript and not a tracks-written log: `tracks` runs
// `claude` interactively (a TUI in a tmux pane), not in `--print`
// mode, so there is no stream-json output for the daemon to capture —
// the per-track `<state_dir>/logs/<id>.jsonl` is never actually
// written. Claude Code, however, persists every session to
//
//	${CLAUDE_CONFIG_DIR:-~/.claude}/projects/<sanitized-cwd>/<session-uuid>.jsonl
//
// where each `type:"assistant"` line carries a `message.usage` block.
// We pin a known session id at spawn (`claude --session-id <uuid>`),
// so a track maps to exactly one transcript; sub-agent (Task tool)
// sidechains share that id and land in the same file.
package usage

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/bluegardenproject/tracks/internal/state"
)

// maxLineBytes caps a single transcript line. Assistant messages embed
// the full response content, so lines routinely exceed bufio.Scanner's
// 64 KiB default — give it generous headroom.
const maxLineBytes = 16 << 20 // 16 MiB

// logLine is the subset of a transcript line we care about. Claude's
// transcript schema is internal and carries many more fields; we
// deliberately decode only these and ignore the rest so the parser
// tolerates schema churn.
type logLine struct {
	Type      string `json:"type"`
	RequestID string `json:"requestId"`
	Message   struct {
		Model string     `json:"model"`
		Usage *usageJSON `json:"usage"`
	} `json:"message"`
}

type usageJSON struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
	// CacheCreation splits cache writes by TTL so each tier can be
	// priced correctly (5-minute writes cost less than 1-hour ones).
	// Absent on older transcripts — then the whole cache-creation
	// count is priced as a 5-minute write.
	CacheCreation *struct {
		Ephemeral5m int64 `json:"ephemeral_5m_input_tokens"`
		Ephemeral1h int64 `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

// ForTrack locates and parses the transcript(s) for a track, returning
// the aggregated usage. A missing transcript is not an error — it
// yields a zero Usage (the track may not have produced any assistant
// turns yet, or Claude hasn't flushed the file).
func ForTrack(sessionID, cwd string) (state.Usage, error) {
	return ParseFiles(Locate(sessionID, cwd))
}

// Locate returns the transcript file(s) for a track.
//
// Primary: glob `projects/*/<sessionID>.jsonl` across every project
// dir — exact, and free of any dependency on how Claude derives a
// project directory name from the cwd. Fallback (e.g. if a Claude
// build doesn't honor --session-id in interactive mode): derive the
// project dir from the worktree cwd and take every transcript in it.
// Because each track has a unique worktree path, that directory holds
// only this track's sessions, so aggregating all of them is correct
// (and also captures a manual re-run in the pane).
func Locate(sessionID, cwd string) []string {
	base, err := projectsDir()
	if err != nil {
		return nil
	}
	if sessionID != "" {
		if matches, _ := filepath.Glob(filepath.Join(base, "*", sessionID+".jsonl")); len(matches) > 0 {
			return matches
		}
	}
	if cwd != "" {
		matches, _ := filepath.Glob(filepath.Join(base, sanitizeCWD(cwd), "*.jsonl"))
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

// ParseFiles sums usage across several transcript files, deduping
// repeated API calls by request id so a line that appears twice can't
// be double-counted.
func ParseFiles(paths []string) (state.Usage, error) {
	var total state.Usage
	seen := map[string]struct{}{}
	for _, p := range paths {
		if err := accumulate(p, &total, seen); err != nil {
			return total, err
		}
	}
	return total, nil
}

// Parse totals the usage in a single transcript file.
func Parse(path string) (state.Usage, error) {
	var total state.Usage
	err := accumulate(path, &total, map[string]struct{}{})
	return total, err
}

func accumulate(path string, total *state.Usage, seen map[string]struct{}) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no transcript yet → contributes nothing
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	for sc.Scan() {
		var line logLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			continue // tolerate a malformed/partial line
		}
		if line.Type != "assistant" || line.Message.Usage == nil {
			continue
		}
		if line.RequestID != "" {
			if _, dup := seen[line.RequestID]; dup {
				continue
			}
			seen[line.RequestID] = struct{}{}
		}
		addMessage(total, line.Message.Model, line.Message.Usage)
	}
	return sc.Err()
}

// addMessage folds one assistant message's usage into the running
// total: token counts sum directly, and cost is the API's per-request
// billing (each tier priced at the message's own model rate, so a
// track whose subagents run Haiku is priced line by line).
func addMessage(total *state.Usage, model string, u *usageJSON) {
	e5m, e1h := u.CacheCreationInputTokens, int64(0)
	if u.CacheCreation != nil {
		e5m, e1h = u.CacheCreation.Ephemeral5m, u.CacheCreation.Ephemeral1h
	}

	total.InputTokens += u.InputTokens
	total.OutputTokens += u.OutputTokens
	total.CacheReadTokens += u.CacheReadInputTokens
	total.CacheCreationTokens += e5m + e1h

	in, out := priceFor(model)
	total.CostUSD += (float64(u.InputTokens)*in +
		float64(u.CacheReadInputTokens)*in*cacheReadMult +
		float64(e5m)*in*cacheWrite5mMult +
		float64(e1h)*in*cacheWrite1hMult +
		float64(u.OutputTokens)*out) / 1e6
}
