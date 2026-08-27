package daemon

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
)

func TestOnlyClaudeTracksHaveAParsableTranscript(t *testing.T) {
	for provider, want := range map[state.Provider]bool{
		state.ProviderClaude: true,
		"":                   true, // every pre-provider record
		state.ProviderCursor: false,
	} {
		if got := hasParsableTranscript(state.Track{Provider: provider}); got != want {
			t.Errorf("provider %q: hasParsableTranscript = %v, want %v", provider, got, want)
		}
	}
}

// The gate has to hold for the whole track lifetime, not just the
// poll: finalizeTrack settles usage once more on exit.
func TestCursorTrackIsNeverCosted(t *testing.T) {
	tr := state.Track{Provider: state.ProviderCursor, SessionID: "08ab06bf-8ec1-495f-90c9-b7523131b824"}
	if hasParsableTranscript(tr) {
		t.Fatal("a Cursor track would be parsed against Claude transcripts and costed at Anthropic rates")
	}
	// Its usage therefore stays zero, which is what makes the dashboard
	// render "—" rather than a number — see internal/usage for why a
	// number would be wrong.
	if !tr.Usage.IsZero() {
		t.Error("fixture is not zero-usage; the assertion below is meaningless")
	}
}
