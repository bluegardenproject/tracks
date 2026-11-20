package cursor

import (
	"encoding/json"
	"strings"
	"unicode"
)

// Stream follows a print-mode turn's `--output-format stream-json
// --stream-partial-output` events: what to show in a pane as they come,
// and the turn's final text.
//
// Cursor writes a turn's text in pieces, and after each segment of it,
// the text between two tool calls, repeats the whole segment once. A
// piece that equals its segment so far is held back: dropped when a tool
// call or the end comes next, shown when more text does.
type Stream struct {
	// segment is the text since the last tool call; held is a piece
	// that may be its repeat.
	segment strings.Builder
	held    string
	// Segments are the turn's texts between its tool calls, once a
	// result event came; Result is its whole text, as Cursor gives it.
	// Failed says the turn ended in an error.
	Segments []string
	Result   string
	Done     bool
	Failed   bool
	// cwd is the turn's folder, from its init event: paths in it are
	// shown relative to it.
	cwd string
}

type streamEvent struct {
	Type     string                     `json:"type"`
	Subtype  string                     `json:"subtype"`
	Cwd      string                     `json:"cwd"`
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

// maxOther bounds a line that isn't an event, shown as it is.
const maxOther = 2000

// Feed reads one line of the stream and returns what to show for it,
// "" for nothing, without terminal controls: the text comes from a model
// that read untrusted code. A line that isn't an event, such as a warning
// the CLI prints, is shown as it is, cut when long.
func (s *Stream) Feed(line string) string {
	return plain(s.feed(line))
}

func (s *Stream) feed(line string) string {
	var e streamEvent
	if json.Unmarshal([]byte(line), &e) != nil || e.Type == "" {
		if len(line) > maxOther {
			return "(a line of the stream too long to show)\n"
		}
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
		if piece == "" {
			return ""
		}
		out := s.release()
		if s.segment.Len() > 0 && strings.TrimSpace(piece) == strings.TrimSpace(s.segment.String()) {
			s.held = piece
			return out
		}
		s.segment.WriteString(piece)
		return out + piece
	case "tool_call":
		if e.Subtype != "started" {
			return ""
		}
		out := "\n→ " + s.relative(toolLine(e.ToolCall)) + "\n"
		if s.segment.Len() == 0 {
			out = out[1:]
		}
		s.held = ""
		s.endSegment()
		return out
	case "result":
		// A piece held back here is dropped: it's the repeat, or, rarely,
		// a last piece that happens to equal its segment.
		s.held = ""
		s.endSegment()
		s.Done, s.Failed, s.Result = true, e.IsError, e.Result
		if s.Failed {
			return "\nThe turn failed: " + oneLine(e.Result, maxOther) + "\n"
		}
		return "\n"
	}
	return ""
}

// endSegment keeps the text since the last tool call as a segment.
func (s *Stream) endSegment() {
	if text := strings.TrimSpace(s.segment.String()); text != "" {
		s.Segments = append(s.Segments, text)
	}
	s.segment.Reset()
}

// release shows a held piece once more text shows it wasn't a repeat.
func (s *Stream) release() string {
	held := s.held
	s.held = ""
	s.segment.WriteString(held)
	return held
}

// relative is line with the turn's folder taken off its paths. macOS
// names a temporary folder with /private in front or without.
func (s *Stream) relative(line string) string {
	for _, dir := range []string{s.cwd, strings.TrimPrefix(s.cwd, "/private")} {
		if dir = strings.TrimSuffix(dir, "/"); dir != "" {
			line = strings.ReplaceAll(line, dir+"/", "")
		}
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

// plain is s without control characters but line ends and tabs, and
// without the characters that reorder text: no escape sequence reaches
// the pane.
func plain(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case unicode.IsControl(r), r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200e, r == 0x200f:
			return -1
		}
		return r
	}, s)
}
