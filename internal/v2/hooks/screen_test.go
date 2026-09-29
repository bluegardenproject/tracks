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
	if _, known := DialogOpen("cursor", question); known {
		t.Error("Cursor's dialogs aren't known yet")
	}
}
