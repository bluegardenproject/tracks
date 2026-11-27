package rpc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/relay"
)

func TestRelaySendParamsCarryTheFields(t *testing.T) {
	b, err := json.Marshal(RelaySendParams{ID: "r", To: "track", Kind: relay.Report, Body: "Done.",
		Fields: relay.Fields{Outcome: relay.Done, Refs: []string{"#12"}}})
	if err != nil || !strings.Contains(string(b), `"outcome":"done","refs":["#12"]`) {
		t.Errorf("marshalled %s, %v", b, err)
	}
	b, err = json.Marshal(RelaySendParams{ID: "r", To: "track", Kind: relay.Report, Body: "Done."})
	if err != nil || strings.Contains(string(b), "outcome") || strings.Contains(string(b), "refs") {
		t.Errorf("without fields, marshalled %s, %v", b, err)
	}
	var p RelaySendParams
	if err := json.Unmarshal([]byte(`{"id":"r","to":"track","kind":"report","body":"Done."}`), &p); err != nil || p.Outcome != "" || p.Refs != nil {
		t.Errorf("a send from before the fields decoded as %+v, %v", p, err)
	}
}
