package hooks

import "testing"

func TestDialogOpen(t *testing.T) {
	permission := ` Bash command
   go test ./...
 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for go commands in /repo
   3. No, and tell Claude what to do differently (esc)`
	question := `☐ Storage
Which database should the cache use?
❯ 1. SQLite
  2. Postgres`
	working := `⏺ Running the tests…
  ⎿  ok  github.com/x/y  0.5s
> `
	for screen, want := range map[string]bool{permission: true, question: true, working: false, "": false} {
		if open, known := DialogOpen("claude", screen); open != want || !known {
			t.Errorf("DialogOpen(claude, %q) = %v, %v; want %v", screen, open, known, want)
		}
	}
	if _, known := DialogOpen("codex", question); known {
		t.Error("an engine without markers isn't known")
	}
}

func TestCursorDialogOpen(t *testing.T) {
	command := `  $ touch /tmp/probe.txt Waiting for approval...

────────────────────────────────────────────────────────
 $  touch /tmp/probe.txt in /repo

 Run this command?
 Not in allowlist: touch
  → Run (once) (y)
    Run Everything (shift+tab)
    Skip & tell the agent what to do instead (esc or n)`
	question := ` ┌──────────────────────────────────────────────┐
 │ Clarifying Questions                         │
 │                                              │
 │ Question 1 of 1                              │
 │ 1. Red or blue?                              │
 │   › [ ] Red                                  │
 │     [ ] Blue                                 │
 │ ↑/↓ option · Enter next/submit · Esc to skip │
 └──────────────────────────────────────────────┘`
	working := `  I ran touch /tmp/probe.txt. Run this command? → Run (once) (y) was
  the approval you gave.
  → Add a follow-up
  Claude Opus 5.5 300K High · 8%`
	plan := ` ┌──────────────────────────────────────┐
 │ Suggested Plan                       │
 │                                      │
 │ 1. Add the flag                      │
 │ 2. Test it                           │
 │ ──────────────────────────────────── │
 │                                      │
 │ Ready to build?                      │
 │                                      │
 │  → 1. Yes, build locally (b)         │
 │    2. Yes, build in cloud (c)        │
 │    3. No, propose changes (p or Esc) │
 │                                      │
 └──────────────────────────────────────┘

`
	longPlan := ` │ 9. Release it                        │
 │ ──────────────────────────────────── │
 │                                      │
 │ Ready to build?                      │
 │                                      │
 │  → 1. Yes, build locally (b)         │
 │    2. No, propose changes (p or Esc) │
 │                                      │
 └──────────────────────────────────────┘`
	accepted := ` │ Plan                                 │
 │ Ready to build?                      │
 │  → 1. Yes, build locally (enter)     │
 │    2. No, propose changes (p or Esc) │
 └──────────────────────────────────────┘

  Adding the flag…
  → Add a follow-up`
	modeSwitch := ` Switch to Agent mode?
 The plan is ready to build.
  → Approve mode switch (y)
    Reject (n or esc)
 auto-rejects when the bar runs out`
	switched := `  SwitchMode → Agent
  → Add a follow-up`
	for screen, want := range map[string]bool{
		command: true, question: true, plan: true, longPlan: true, modeSwitch: true,
		working: false, accepted: false, switched: false, "": false,
	} {
		if open, known := DialogOpen("cursor", screen); open != want || !known {
			t.Errorf("DialogOpen(cursor, %q) = %v, %v; want %v", screen, open, known, want)
		}
	}
	if !ScreenOpens("cursor") || ScreenOpens("claude") {
		t.Error("only Cursor's dialogs are told open by the screen")
	}
}
