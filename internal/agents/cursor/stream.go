package cursor

import (
	"encoding/json"
	"strings"
)

// Stream follows a print-mode turn's `--output-format stream-json
// --stream-partial-output` events: what to show in a pane as they come,
// and the turn's final text.
type Stream struct {
	// text is what the assistant has written since the last tool call,
	// so the full message Cursor repeats at the end isn't shown twice.
	text strings.Builder
	// Result is the turn's final text, once a result event came; Failed
	// says the turn ended in an error.
	Result string
	Done   bool
	Failed bool
	// cwd is the turn's folder, from its init event: paths in it are
	// shown relative to it.
	cwd string
}

type streamEvent struct {
	Type     string                     `json:"type"`
	Cwd      string                     `json:"cwd"`
	Subtype  string                     `json:"subtype"`
	IsError  bool                       `json:"is_error"`
	Result   string                     `json:"result"`
	ToolCall map[string]json.RawMessage `json:"tool_call"`
	Message  struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

// Feed reads one line of the stream and returns what to show for it,
// "" for nothing. A line that isn't an event, such as a warning the CLI
// prints, is shown as it is.
func (s *Stream) Feed(line string) string {
	var e streamEvent
	if json.Unmarshal([]byte(line), &e) != nil || e.Type == "" {
		return line + "\n"
	}
	switch e.Type {
	case "system":
		s.cwd = e.Cwd
	case "assistant":
		var b strings.Builder
		for _, c := range e.Message.Content {
			if c.Type == "text" {
				b.WriteString(c.Text)
			}
		}
		piece := b.String()
		if piece == "" || s.text.Len() > 0 && piece == s.text.String() {
			return ""
		}
		s.text.WriteString(piece)
		return piece
	case "tool_call":
		if e.Subtype != "started" {
			return ""
		}
		out := "\n→ " + s.relative(toolLine(e.ToolCall)) + "\n"
		if s.text.Len() == 0 {
			out = out[1:]
		}
		s.text.Reset()
		return out
	case "result":
		s.Done, s.Failed, s.Result = true, e.IsError, e.Result
		if s.Failed {
			return "\nThe turn failed: " + e.Result + "\n"
		}
		return "\n"
	}
	return ""
}

// relative is line with the turn's folder taken off its paths. macOS
// names a temporary folder with /private in front or without.
func (s *Stream) relative(line string) string {
	if s.cwd == "" {
		return line
	}
	for _, dir := range []string{s.cwd, strings.TrimPrefix(s.cwd, "/private")} {
		line = strings.ReplaceAll(line, strings.TrimSuffix(dir, "/")+"/", "")
	}
	return line
}

// toolLine is a tool call in a few words: its name and what it's on.
func toolLine(call map[string]json.RawMessage) string {
	for key, raw := range call {
		name, ok := strings.CutSuffix(key, "ToolCall")
		if !ok {
			continue
		}
		var c struct {
			Args map[string]any `json:"args"`
		}
		_ = json.Unmarshal(raw, &c)
		for _, arg := range []string{"path", "pattern", "globPattern", "query", "command", "url"} {
			if v, ok := c.Args[arg].(string); ok && v != "" {
				return name + " " + oneLine(v, 100)
			}
		}
		return name
	}
	return "a tool"
}

// oneLine is s on one line, cut at max runes.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
