package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/rpc"
)

func TestRunHook(t *testing.T) {
	tests := []struct {
		engine, input string
		reported      string // "" when nothing is reported
		reply         string
		logged        bool
	}{
		{"claude", `{"hook_event_name":"PermissionRequest"}`, "agent.waiting", "", false},
		{"claude", `{"hook_event_name":"SessionStart"}`, "", "", false},
		{"cursor", `{"hook_event_name":"beforeSubmitPrompt"}`, "agent.working", `{"continue":true}`, false},
		{"cursor", `not json`, "", "{}", true},
		{"cursor", `{"hook_event_name":"afterAgentResponse","text":"TRACKS_PR_URL=https://github.com/a/b/pull/1"}`,
			" [https://github.com/a/b/pull/1]", "{}", false},
		{"claude", `{"hook_event_name":"Stop","last_assistant_message":"TRACKS_PR_URL=https://github.com/a/b/pull/1"}`,
			"agent.working [https://github.com/a/b/pull/1]", "", false},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		var got []string
		var logs []string
		report := func(_ context.Context, p rpc.ReportParams) error {
			r := p.ID + " " + p.Event
			if len(p.PRs) > 0 {
				r += fmt.Sprint(" ", p.PRs)
			}
			got = append(got, r)
			return nil
		}
		logf := func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
		runHook(context.Background(), strings.NewReader(tt.input), &out, tt.engine, "t1", report, logf)
		want := []string(nil)
		if tt.reported != "" {
			want = []string{"t1 " + tt.reported}
		}
		if fmt.Sprint(got) != fmt.Sprint(want) || out.String() != tt.reply || (len(logs) > 0) != tt.logged {
			t.Errorf("%s %s: reported %v, replied %q, logged %v", tt.engine, tt.input, got, out.String(), logs)
		}
	}
}

func TestRunHookWithTheDaemonDown(t *testing.T) {
	var out bytes.Buffer
	var logs []string
	down := func(context.Context, rpc.ReportParams) error { return errors.New("connection refused") }
	runHook(context.Background(), strings.NewReader(`{"hook_event_name":"beforeSubmitPrompt"}`), &out, "cursor", "t1", down,
		func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) })
	if out.String() != `{"continue":true}` || len(logs) != 1 || !strings.Contains(logs[0], "connection refused") {
		t.Errorf("replied %q, logged %v; want the reply anyway and the error logged", out.String(), logs)
	}
}
