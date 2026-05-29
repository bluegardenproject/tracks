package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

// Event is anything the daemon wants to react to from Claude's
// stream-json output. The interface is open so unknown event types
// from the future are simply ignored — they fall through to the
// fallback no-op event and never crash the parser.
type Event interface {
	// Type is the discriminator from the JSON payload (e.g. "tool_use").
	Type() string
}

// AssistantText is one chunk of assistant prose.
type AssistantText struct{ Text string }

func (AssistantText) Type() string { return "assistant_text" }

// ToolUse signals Claude invoked a tool. Name is the tool name
// (e.g. "Edit", "Bash"). Input is left as raw JSON so we don't
// commit to a schema we can't guarantee stable.
type ToolUse struct {
	Name  string
	Input json.RawMessage
}

func (ToolUse) Type() string { return "tool_use" }

// ProcessExit isn't from the stream-json itself — the daemon
// synthesizes it when the Claude process terminates. Surfacing it
// through the same Event channel keeps consumer code simple.
type ProcessExit struct {
	Code int
	Err  error
}

func (ProcessExit) Type() string { return "process_exit" }

// PRMarker is emitted when the assistant prose contains a
// TRACKS_PR_URL=<value> line. Empty URL means "no PR".
type PRMarker struct{ URL string }

func (PRMarker) Type() string { return "pr_marker" }

// trackingPrefix is the marker we inject into every prompt asking
// Claude to emit at finish-time. Picking it up from the log is
// vastly more reliable than scraping arbitrary prose for PR URLs.
const trackingPrefix = "TRACKS_PR_URL="

// TailLog opens path and emits events parsed from each line. It
// follows the file (tail -F semantics) until ctx is cancelled or
// the optional done channel fires. Send events on out.
//
// The parser is intentionally permissive: any JSON payload that
// doesn't decode into the known schema is dropped silently. Any
// plain-text line (e.g. setup messages on stderr that got merged
// into the same file) is parsed for the PR marker but otherwise
// ignored. This is the "stream-json is enrichment, not authority"
// design from the plan.
func TailLog(ctx context.Context, path string, out chan<- Event) error {
	f, err := openWithRetry(ctx, path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReader(f)
	for {
		line, err := r.ReadString('\n')
		if line != "" {
			emitFromLine(strings.TrimRight(line, "\n"), out)
		}
		if err == nil {
			continue
		}
		if err != io.EOF {
			return err
		}
		// EOF: wait for more data or for ctx cancellation.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// openWithRetry waits up to ~5s for the log file to appear. Claude
// can take a moment between fork and first write; not retrying
// would race the spawn.
func openWithRetry(ctx context.Context, path string) (*os.File, error) {
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := os.Open(path)
		if err == nil {
			return f, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// emitFromLine inspects one log line and dispatches Event(s) to out.
// Unknown JSON shapes are dropped. Plain text is scanned for the PR
// marker.
func emitFromLine(line string, out chan<- Event) {
	if line == "" {
		return
	}
	// Look for the PR marker first — it can appear in stream-json
	// "text" fields or in plain text, so we check both forms.
	if i := strings.Index(line, trackingPrefix); i >= 0 {
		tail := line[i+len(trackingPrefix):]
		// Stop at the first quote, comma, brace, or whitespace —
		// those reliably terminate the marker even inside JSON.
		end := strings.IndexAny(tail, `",}` + "\n \t")
		if end < 0 {
			end = len(tail)
		}
		url := strings.TrimSpace(tail[:end])
		if url == "none" {
			out <- PRMarker{URL: ""}
		} else if url != "" {
			out <- PRMarker{URL: url}
		}
	}

	// stream-json lines are JSON objects with a discriminator we
	// don't know the exact schema of. Try a permissive decode.
	if !strings.HasPrefix(strings.TrimSpace(line), "{") {
		return
	}
	var blob map[string]any
	if err := json.Unmarshal([]byte(line), &blob); err != nil {
		return
	}
	// Look for common shapes. Claude's stream-json emits envelopes
	// with `type` and content. We probe for the shapes we care
	// about; anything else is silently ignored.
	switch t, _ := blob["type"].(string); t {
	case "assistant", "assistant_message":
		if text := extractText(blob); text != "" {
			out <- AssistantText{Text: text}
		}
	case "tool_use":
		name, _ := blob["name"].(string)
		raw, _ := json.Marshal(blob["input"])
		out <- ToolUse{Name: name, Input: raw}
	}
}

// extractText walks common shapes for assistant text in stream-json
// payloads. We probe top-level "text", then "message.content[].text"
// arrays. If neither matches we return empty.
func extractText(blob map[string]any) string {
	if t, ok := blob["text"].(string); ok && t != "" {
		return t
	}
	if msg, ok := blob["message"].(map[string]any); ok {
		if content, ok := msg["content"].([]any); ok {
			var b strings.Builder
			for _, c := range content {
				if cm, ok := c.(map[string]any); ok {
					if t, ok := cm["text"].(string); ok {
						if b.Len() > 0 {
							b.WriteString("\n")
						}
						b.WriteString(t)
					}
				}
			}
			return b.String()
		}
	}
	return ""
}
