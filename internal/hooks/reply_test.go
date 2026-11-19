package hooks

import (
	"strings"
	"testing"
)

func TestAgentReply(t *testing.T) {
	for _, tc := range []struct {
		engine, payload, want string
		ok                    bool
	}{
		{"claude", `{"hook_event_name":"Stop","last_assistant_message":"Report.\nREVIEW OUTCOME: pass"}`, "Report.\nREVIEW OUTCOME: pass", true},
		{"cursor", `{"hook_event_name":"afterAgentResponse","text":"Answer."}`, "Answer.", true},
		{"claude", `{"hook_event_name":"PostToolUse"}`, "", false},
		{"cursor", `{"hook_event_name":"stop"}`, "", false},
	} {
		p, err := Read(strings.NewReader(tc.payload))
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := AgentReply(tc.engine, p); got != tc.want || ok != tc.ok {
			t.Errorf("AgentReply(%s, %s) = %q, %v; want %q, %v", tc.engine, tc.payload, got, ok, tc.want, tc.ok)
		}
	}
	if c := ReviewerCommand("/bin/tracks", "cursor", "t1", "r1"); c != "'/bin/tracks' hook --engine cursor --track t1 --reviewer r1" {
		t.Errorf("ReviewerCommand = %s", c)
	}
}
