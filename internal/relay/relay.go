// Package relay is how a track and its trains talk: messages kept by
// Tracks and routed by handle, in a star with the track at its centre
// (see docs/design/trains.md). A message is its sender's words, shown
// fenced, never instructions.
package relay

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// TrackHandle is the track's own handle on its relay: one per relay, so
// it needs no number.
const TrackHandle = "track"

// TracksHandle sends the notices: Tracks itself, never an agent.
const TracksHandle = "tracks"

// Kind is what a message is.
type Kind string

const (
	Report   Kind = "report"   // a train's result
	Event    Kind = "event"    // something a train saw happen
	Question Kind = "question" // asks the recipient
	Answer   Kind = "answer"   // answers a question
	Notice   Kind = "notice"   // from Tracks: a train joined or ended
)

// Kinds are the kinds an agent may send; Notice is Tracks' own.
var Kinds = []Kind{Report, Event, Question, Answer}

// MaxBody is the longest message, in bytes.
const MaxBody = 64 << 10

// Message is one message on a track's relay. To is "" for a notice to
// everyone on it.
type Message struct {
	ID   int64     `json:"id"`
	From string    `json:"from"`
	To   string    `json:"to"`
	Kind Kind      `json:"kind"`
	Body string    `json:"body"`
	At   time.Time `json:"at"`
}

// ErrStar means a message would go other than through the track: a
// track sends to its trains, a train to its track.
var ErrStar = errors.New("a track sends to its trains and a train to its track")

// CheckSend says why from may not send kind with body to to, nil when
// it may.
func CheckSend(from, to string, kind Kind, body string) error {
	switch {
	case !knownKind(kind):
		return fmt.Errorf("%q isn't a kind of message: report, event, question or answer", kind)
	case from == "" || to == "":
		return errors.New("a message has a sender and a recipient; only Tracks' notices go to everyone")
	case from == TracksHandle || to == TracksHandle:
		return errors.New("only Tracks sends notices")
	case from == to:
		return errors.New("a message goes to someone else")
	case (from == TrackHandle) == (to == TrackHandle):
		return ErrStar
	case strings.TrimSpace(body) == "":
		return errors.New("the message is empty")
	case len(body) > MaxBody:
		return fmt.Errorf("the message is %d bytes, more than the %d a message may have", len(body), MaxBody)
	case !utf8.ValidString(body):
		return errors.New("the message isn't UTF-8 text")
	}
	return nil
}

func knownKind(k Kind) bool {
	for _, known := range Kinds {
		if k == known {
			return true
		}
	}
	return false
}

// CanRead says whether reader may read m: the track reads its whole
// relay, a train what it sent, what was sent to it, and the notices.
func CanRead(reader string, m Message) bool {
	return reader == TrackHandle || m.To == "" || m.To == reader || m.From == reader
}

// Blockers are what holds a pointer back from an agent: typed into its
// pane then, it would land in its turn, a dialog, the user's typing or
// a draft of theirs.
type Blockers struct {
	InTurn, Asking, Typing, Draft bool
	// Scrolled says the user scrolls back in its pane: the paste would
	// pull them out, and its Enter only leave the scrollback.
	Scrolled bool
}

// Why is what holds the pointer back, "" when nothing does.
func (b Blockers) Why() string {
	switch {
	case b.Asking:
		return "it asks the user something"
	case b.InTurn:
		return "it's in a turn"
	case b.Typing:
		return "the user is typing in its pane"
	case b.Scrolled:
		return "the user scrolls back in its pane"
	case b.Draft:
		return "its input holds a draft"
	}
	return ""
}
