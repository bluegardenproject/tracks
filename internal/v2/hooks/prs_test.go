package hooks

import (
	"slices"
	"strings"
	"testing"
)

func TestPRs(t *testing.T) {
	const pr = "https://github.com/acme/web/pull/42"
	tests := []struct {
		name, engine, input string
		want                []string
	}{
		{"claude gh pr create", "claude", `{"hook_event_name":"PostToolUse","tool_name":"Bash",
			"tool_input":{"command":"gh pr create --fill"},
			"tool_response":{"stdout":"Creating pull request\n` + pr + `\n","stderr":""}}`, []string{pr}},
		{"claude other command", "claude", `{"hook_event_name":"PostToolUse","tool_name":"Bash",
			"tool_input":{"command":"gh pr view 42"},"tool_response":{"stdout":"` + pr + `"}}`, nil},
		{"claude other tool", "claude", `{"hook_event_name":"PostToolUse","tool_name":"Read",
			"tool_input":{"file_path":"gh pr create"},"tool_response":"` + pr + `"}`, nil},
		{"claude stop", "claude", `{"hook_event_name":"Stop","last_assistant_message":
			"Opened it.\n\n` + "`TRACKS_PR_URL=" + pr + "`" + `\nTRACKS_PR_URL=https://github.com/acme/api/pull/3/files\nTRACKS_PR_URL=https://github.com/acme/web/pull/42"}`,
			[]string{pr, "https://github.com/acme/api/pull/3"}},
		{"claude stop quoting it", "claude", `{"hook_event_name":"Stop","last_assistant_message":
			"Print TRACKS_PR_URL=` + pr + ` when done."}`, nil},
		{"claude stop without a message", "claude", `{"hook_event_name":"Stop","last_assistant_message":{"x":1}}`, nil},
		{"cursor gh pr create", "cursor", `{"hook_event_name":"afterShellExecution",
			"command":"git push && gh pr create --draft","output":"` + pr + `\n","duration":1200}`, []string{pr}},
		{"cursor answer", "cursor", `{"hook_event_name":"afterAgentResponse","text":"Done.\nTRACKS_PR_URL=` + pr + `"}`, []string{pr}},
		{"cursor other shell", "cursor", `{"hook_event_name":"afterShellExecution","command":"ls","output":"` + pr + `"}`, nil},
		{"cursor event of claude's", "cursor", `{"hook_event_name":"Stop","last_assistant_message":"TRACKS_PR_URL=` + pr + `"}`, nil},
	}
	for _, tt := range tests {
		p, err := Read(strings.NewReader(tt.input))
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if got := PRs(tt.engine, p); !slices.Equal(got, tt.want) {
			t.Errorf("%s: PRs = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestPRPayloadsStillMapEvents(t *testing.T) {
	p, err := Read(strings.NewReader(`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":[1],"tool_response":7}`))
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := Event("claude", p); !ok || e != "agent.working" {
		t.Errorf("an odd tool_input still means working: %q, %v", e, ok)
	}
}
