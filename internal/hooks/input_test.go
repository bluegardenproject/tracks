package hooks

import "testing"

// The screens are as the spike captured them (docs/design/trains.md,
// Spike): both engines draw the placeholder dim, a draft in plain text.
const (
	ruleLine    = "────────────────────────────────────────────────────────────"
	claudeEmpty = ruleLine + "\n\x1b[39m❯ \x1b[2mTry \"refactor <filepath>\"\x1b[0m\n" + ruleLine + "\n  ➜  repo git:(master)\n  ⏵⏵ auto mode on"
	claudeGhost = ruleLine + "\n\x1b[39m❯ \x1b[2mcreate a.txt with hi using bash instead\x1b[0m\n" + ruleLine
	claudeDraft = ruleLine + "\n\x1b[39m❯ hello draft\n" + ruleLine
	claudeLines = ruleLine + "\n\x1b[39m❯ \n  a second line\n" + ruleLine
	claudeTrust = ruleLine + "\n Accessing workspace:\n ❯ No, exit\n   Yes, I trust this folder\n Enter to confirm · Esc to cancel"
	cursorEmpty = "  Cursor Agent\n \x1b[48;2;21;21;21m \x1b[2m→ Plan, search, build anything\x1b[0m\x1b[48;2;21;21;21m      \x1b[49m\n  Gemini 4"
	cursorDraft = "  Cursor Agent\n \x1b[48;2;21;21;21m → hello draft            \x1b[49m\n  Gemini 4"
)

func TestInputEmpty(t *testing.T) {
	for _, tc := range []struct {
		name, engine, screen string
		empty, known         bool
	}{
		{"claude, placeholder", "claude", claudeEmpty, true, true},
		{"claude, a suggestion", "claude", claudeGhost, true, true},
		{"claude, a draft", "claude", claudeDraft, false, true},
		{"claude, a draft's second line", "claude", claudeLines, false, true},
		{"claude, a dialog", "claude", claudeTrust, false, false},
		{"claude, nothing", "claude", "", false, false},
		{"cursor, placeholder", "cursor", cursorEmpty, true, true},
		{"cursor, a draft", "cursor", cursorDraft, false, true},
		{"cursor, nothing", "cursor", "Cursor Agent", false, false},
		{"an engine Tracks can't read", "other", claudeEmpty, false, false},
	} {
		empty, known := InputEmpty(tc.engine, tc.screen)
		if empty != tc.empty || known != tc.known {
			t.Errorf("%s: empty %v known %v, want %v %v", tc.name, empty, known, tc.empty, tc.known)
		}
	}
}

func TestSGRDim(t *testing.T) {
	for _, tc := range []struct {
		params    string
		was, want bool
	}{
		{"2", false, true}, {"22", true, false}, {"0", true, false}, {"", true, false},
		{"48;2;21;21;21", false, false}, {"38;5;2", false, false}, {"1;2", false, true}, {"39", true, true},
	} {
		if got := sgrDim(tc.params, tc.was); got != tc.want {
			t.Errorf("sgrDim(%q, %v) = %v", tc.params, tc.was, got)
		}
	}
}

func TestTurn(t *testing.T) {
	for _, tc := range []struct {
		engine string
		p      Payload
		want   string
	}{
		{"claude", Payload{Event: "UserPromptSubmit"}, TurnStart},
		{"claude", Payload{Event: "Stop"}, TurnEnd},
		{"claude", Payload{Event: "StopFailure"}, TurnEnd},
		{"claude", Payload{Event: "Stop", Agent: "sub"}, ""},
		{"claude", Payload{Event: "PostToolUse"}, ""},
		{"cursor", Payload{Event: "beforeSubmitPrompt"}, TurnStart},
		{"cursor", Payload{Event: "stop"}, TurnEnd},
		{"cursor", Payload{Event: "postToolUse"}, ""},
		{"other", Payload{Event: "Stop"}, ""},
	} {
		if got := Turn(tc.engine, tc.p); got != tc.want {
			t.Errorf("Turn(%s, %s/%s) = %q, want %q", tc.engine, tc.p.Event, tc.p.Agent, got, tc.want)
		}
	}
}
