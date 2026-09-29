package hooks

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/bluegardenproject/tracks/internal/v2/track"
)

var (
	// createdPR is the link `gh pr create` prints.
	createdPR = regexp.MustCompile(`https://github\.com/[\w.-]+/[\w.-]+/pull/\d+`)
	// announcedPR is a TRACKS_PR_URL= line, as the agents are asked to
	// write, in or out of Markdown.
	announcedPR = regexp.MustCompile("(?m)^[\\s>*`]*TRACKS_PR_URL=(https://github\\.com/[\\w.-]+/[\\w.-]+/pull/\\d+)")
)

// PRs are the pull request links p from engine's hook reports: the
// output of a `gh pr create`, and the TRACKS_PR_URL= lines of the
// agent's answer.
func PRs(engine string, p Payload) []string {
	switch {
	case engine == "claude" && p.Event == "PostToolUse" && p.Tool == "Bash":
		var in struct{ Command string }
		var out struct{ Stdout string }
		_ = json.Unmarshal(p.ToolInput, &in)
		_ = json.Unmarshal(p.ToolResponse, &out)
		return created(in.Command, out.Stdout)
	case engine == "claude" && p.Event == "Stop":
		return announced(text(p.LastMessage))
	case engine == "cursor" && p.Event == "afterShellExecution":
		return created(text(p.Command), text(p.Output))
	case engine == "cursor" && p.Event == "afterAgentResponse":
		return announced(text(p.Text))
	}
	return nil
}

func created(command, output string) []string {
	if !strings.Contains(command, "gh pr create") {
		return nil
	}
	return unique(createdPR.FindAllString(output, -1))
}

func announced(message string) []string {
	var urls []string
	for _, m := range announcedPR.FindAllStringSubmatch(message, -1) {
		urls = append(urls, m[1])
	}
	return unique(urls)
}

// unique keeps the first of each PR, in its one form.
func unique(urls []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, u := range urls {
		if pr, ok := track.ParsePR(u); ok && !seen[pr.URL] {
			seen[pr.URL] = true
			out = append(out, pr.URL)
		}
	}
	return out
}

// text is raw as a string, "" when it's something else.
func text(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}
