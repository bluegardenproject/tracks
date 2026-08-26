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
	Type string `json:"type"`
	// IsSidechain marks a sub-agent (Task tool) turn. Those share the
	// session id and land in the same transcript, so they count toward
	// cost but must not be mistaken for the session's own model.
	IsSidechain bool   `json:"isSidechain"`
	Timestamp   string `json:"timestamp"`
	RequestID   string `json:"requestId"`
	Message     struct {
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

// Totals is everything one transcript scan yields.
type Totals struct {
	// Usage is the billed token spend and cost.
	Usage state.Usage

	// Model is the model id of the most recent main-chain assistant turn
	// — i.e. whatever `/model` last selected, without tracks having to be
	// told. Sub-agent turns are excluded: a Haiku reviewer subagent must
	// not make the session look like it switched to Haiku. Empty until
	// the first assistant turn lands.
	Model string

	// SubagentModel is the model of the most recent *sub-agent* turn (a
	// Task tool sidechain). Reported separately rather than folded in,
	// because "the track is on Opus but its subagents run Haiku" is the
	// interesting fact and either half alone hides it. Empty when no
	// sub-agent has taken a turn.
	//
	// Both fields are last-one-wins: a session that switched models
	// mid-flight reports what it is on now, not what it started on.
	SubagentModel string
}

// ForTrack locates and parses the transcript(s) for a track, returning
// the aggregated totals. A missing transcript is not an error — it
// yields zero totals (the track may not have produced any assistant
// turns yet, or Claude hasn't flushed the file).
func ForTrack(sessionID, cwd string) (Totals, error) {
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
func ParseFiles(paths []string) (total Totals, _ error) {
	// models carries the newest turn found on each chain so far; see
	// latestModels.observe for how "newest" is decided.
	var models latestModels
	// Only total is named — the defer needs it. Leaving the error result
	// unnamed keeps the `if err := accumulate(...)` below from shadowing a
	// named result, which is the silent no-op this defer invites.
	//
	// Deferred so a mid-scan error returns the same shape as success:
	// whatever was accumulated, models included. Assigning after the loop
	// instead would hand an erroring caller partial Usage with the models
	// blanked — a trap, since the Usage half is accumulated in place and
	// looks complete.
	defer func() { total.Model, total.SubagentModel = models.main, models.sub }()
	seen := map[string]struct{}{}
	for _, p := range paths {
		if err := accumulate(p, &total, &models, seen); err != nil {
			return total, err
		}
	}
	return total, nil
}

// latestModels is the newest model seen on each chain during one scan,
// with the timestamp it came from so later files can't lose to earlier
// ones.
type latestModels struct {
	main, mainAt string
	sub, subAt   string
}

// observe folds one assistant turn into the tracker, per chain.
//
// Transcript timestamps are fixed-precision UTC
// (YYYY-MM-DDTHH:MM:SS.mmmZ), so lexical order is chronological order and
// a string compare is enough. It has to be a comparison rather than "last
// line wins" because Locate can return several files and scans them in
// glob order, not chronological order. If the format ever gains a numeric
// offset or variable-width fractions, this misorders silently — parse
// with time.RFC3339 at that point. Equal timestamps resolve to the later
// line within a file and to glob order across files; sub-second stamps
// make that tie rare enough not to engineer around.
//
// The two chains are compared only against themselves, so a sub-agent
// turn can never advance the main model however late it lands.
// Placeholders are ignored on both (see isRealModel).
func (l *latestModels) observe(model, timestamp string, sidechain bool) {
	if !isRealModel(model) {
		return
	}
	if sidechain {
		if timestamp >= l.subAt {
			l.subAt, l.sub = timestamp, model
		}
		return
	}
	if timestamp >= l.mainAt {
		l.mainAt, l.main = timestamp, model
	}
}

// Parse totals a single transcript file.
func Parse(path string) (Totals, error) {
	return ParseFiles([]string{path})
}

func accumulate(path string, total *Totals, models *latestModels, seen map[string]struct{}) error {
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
		if line.Type != "assistant" {
			continue
		}
		models.observe(line.Message.Model, line.Timestamp, line.IsSidechain)
		if line.Message.Usage == nil {
			continue
		}
		if line.RequestID != "" {
			if _, dup := seen[line.RequestID]; dup {
				continue
			}
			seen[line.RequestID] = struct{}{}
		}
		addMessage(&total.Usage, line.Message.Model, line.Message.Usage)
	}
	return sc.Err()
}

// isRealModel reports whether a transcript's model field names an actual
// model. Claude Code writes placeholders like "<synthetic>" on an
// interrupt or an API error; those lines are main-chain and carry a real
// timestamp, so without this check one of them becomes the track's
// reported model until the next genuine turn lands.
//
// Placeholders are rejected by their shape rather than real ids being
// allow-listed by a "claude-" prefix: gateway-hosted ids don't carry it
// (Bedrock uses anthropic.claude-…, Vertex claude-…@…), and an
// allow-list would show them as "no model" forever.
func isRealModel(model string) bool {
	m := strings.TrimSpace(model)
	return m != "" && !strings.HasPrefix(m, "<")
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
