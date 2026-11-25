package relay

import (
	"errors"
	"fmt"
)

// PairLimit is how many messages two side tracks of one main track may
// send each other, both ways together. It never resets: past it, they
// go through their main track.
const PairLimit = 5

// CheckPeerSend says why side track from may not send kind with body to
// to, a side track of the same main track, when sent messages already
// passed between them; nil when it may. Side tracks only ask each other
// and answer: tasks and reports go through their main track.
func CheckPeerSend(from, to string, kind Kind, body string, sent int) error {
	switch {
	case !knownKind(kind):
		return fmt.Errorf("%q isn't a kind of message: question or answer, to another side track", kind)
	case from == "" || to == "":
		return errors.New("a message has a sender and a recipient")
	case reserved(from) || reserved(to):
		return errors.New("side tracks send each other, never as or to @track, @tracks or @github")
	case from == to:
		return errors.New("a message goes to someone else")
	case kind != Question && kind != Answer:
		return fmt.Errorf("side tracks send each other questions and answers only: send your %s to your main track, --to track", kind)
	case sent >= PairLimit:
		return fmt.Errorf("@%s and @%s have sent each other %d messages, the most two side tracks may: go through your main track, --to track",
			from, to, PairLimit)
	}
	return checkBody(body)
}

// Between counts the messages in msgs that a and b sent each other,
// either way.
func Between(msgs []Message, a, b string) int {
	n := 0
	for _, m := range msgs {
		if m.From == a && m.To == b || m.From == b && m.To == a {
			n++
		}
	}
	return n
}

// reserved reports whether h is a handle no side track has.
func reserved(h string) bool { return h == TrackHandle || h == TracksHandle || h == GitHubHandle }
