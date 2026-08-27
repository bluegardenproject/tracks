package state

import (
	"encoding/json"
	"testing"
)

// The zero value has to mean Claude, or every track written before
// providers existed would decode as "no provider" and fail to spawn.
func TestEmptyProviderResolvesToClaude(t *testing.T) {
	if got := Provider("").Resolved(); got != ProviderClaude {
		t.Errorf("empty provider resolved to %q, want %q", got, ProviderClaude)
	}
	if !Provider("").Valid() {
		t.Error("empty provider should be valid — it is what old records carry")
	}
	if got := Provider("").Label(); got != "Claude Code" {
		t.Errorf("empty provider labelled %q", got)
	}
}

func TestProviderValidity(t *testing.T) {
	for _, p := range Providers() {
		if !p.Valid() {
			t.Errorf("%q is offered by Providers() but not Valid()", p)
		}
		if p.Label() == "" {
			t.Errorf("%q has no label", p)
		}
	}
	for _, bad := range []Provider{"cursur", "Claude", "codex", "cursor "} {
		if bad.Valid() {
			t.Errorf("%q accepted as a provider", bad)
		}
	}
}

// A record from before providers existed must decode as Claude.
//
// Note an explicit ProviderClaude *is* written to disk: omitempty only
// elides the zero value, and "claude" is not it. That is harmless — it
// round-trips — but it does mean new Claude records carry a key old
// ones don't.
func TestProviderRoundTrip(t *testing.T) {
	var old Track
	if err := json.Unmarshal([]byte(`{"id":"t1","slug":"s","status":"done"}`), &old); err != nil {
		t.Fatal(err)
	}
	if old.Provider.Resolved() != ProviderClaude {
		t.Errorf("pre-provider record resolved to %q", old.Provider.Resolved())
	}

	blob, err := json.Marshal(Track{ID: "t1", Provider: ProviderClaude})
	if err != nil {
		t.Fatal(err)
	}
	// omitempty only elides the zero value, so an explicit "claude" is
	// written. That is fine — it just must round-trip.
	var back Track
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatal(err)
	}
	if back.Provider.Resolved() != ProviderClaude {
		t.Errorf("round-tripped to %q", back.Provider)
	}

	cursor, err := json.Marshal(Track{ID: "t2", Provider: ProviderCursor})
	if err != nil {
		t.Fatal(err)
	}
	var backC Track
	if err := json.Unmarshal(cursor, &backC); err != nil {
		t.Fatal(err)
	}
	if backC.Provider != ProviderCursor {
		t.Errorf("cursor track round-tripped to %q", backC.Provider)
	}
}
