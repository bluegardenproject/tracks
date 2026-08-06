package dashboard

import (
	"testing"

	"github.com/bluegardenproject/tracks/internal/state"
)

// docSectionsOff reports the narrowing only: a full review renders no
// "off" segment at all, and the switches are meaningless off a doc track.
func TestDocSectionsOff(t *testing.T) {
	cases := []struct {
		name       string
		kind       state.Kind
		claimCheck bool
		opinion    bool
		want       string
	}{
		{"full review", state.KindDoc, false, false, ""},
		{"claim check off", state.KindDoc, true, false, "claim-check"},
		{"opinion off", state.KindDoc, false, true, "opinion"},
		{"both off", state.KindDoc, true, true, "opinion,claim-check"},
		{"flags on a work track", state.KindWork, true, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := docSectionsOff(state.Track{
				Kind:              tc.kind,
				DocSkipClaimCheck: tc.claimCheck,
				DocSkipOpinion:    tc.opinion,
			})
			if got != tc.want {
				t.Errorf("docSectionsOff() = %q, want %q", got, tc.want)
			}
		})
	}
}
