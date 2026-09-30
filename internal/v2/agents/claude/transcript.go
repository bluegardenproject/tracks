package claude

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/bluegardenproject/tracks/internal/usage"
)

// maxCarried bounds the reply a promoted track starts with: a plan is
// rarely longer, and the prompt is also the track's details.
const maxCarried = 20_000

// LastReply is what session's last turn ended with, for a promoted
// track to go on from: the plan of an ExitPlanMode in it, else its
// text. "" when the transcript can't be found or has neither.
func LastReply(session string) string {
	var last string
	for _, path := range usage.Locate(session, "") {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		if reply := lastReply(f); reply != "" {
			last = reply
		}
		f.Close()
	}
	return carried(last)
}

type transcriptLine struct {
	Type    string `json:"type"`
	IsMeta  bool   `json:"isMeta"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
	Name  string `json:"name"`
	Input struct {
		Plan string `json:"plan"`
	} `json:"input"`
}

// lastReply reads a transcript turn by turn. A turn starts with a
// prompt from the user; tool results don't start one.
func lastReply(r io.Reader) string {
	var last, plan string
	var text []string
	endTurn := func() {
		switch {
		case plan != "":
			last = plan
		case len(text) > 0:
			last = strings.Join(text, "\n\n")
		}
		plan, text = "", nil
	}
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scan.Scan() {
		var line transcriptLine
		if json.Unmarshal(scan.Bytes(), &line) != nil || line.IsMeta {
			continue
		}
		var prompt string
		if json.Unmarshal(line.Message.Content, &prompt) == nil {
			if line.Type == "user" {
				endTurn()
			}
			continue
		}
		var blocks []contentBlock
		if json.Unmarshal(line.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			switch {
			case line.Type == "user" && b.Type == "text":
				endTurn()
			case line.Type == "assistant" && b.Type == "text" && strings.TrimSpace(b.Text) != "":
				text = append(text, strings.TrimSpace(b.Text))
			case line.Type == "assistant" && b.Type == "tool_use" && b.Name == "ExitPlanMode" && b.Input.Plan != "":
				plan = strings.TrimSpace(b.Input.Plan)
			}
		}
	}
	endTurn()
	return last
}

// carried is reply cut to maxCarried bytes, on a character boundary,
// saying so.
func carried(reply string) string {
	if len(reply) <= maxCarried {
		return reply
	}
	cut := maxCarried
	for cut > 0 && !utf8.RuneStart(reply[cut]) {
		cut--
	}
	return reply[:cut] + "\n\n[cut at 20 KB]"
}
