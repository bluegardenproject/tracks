package side

import (
	"strings"

	"github.com/bluegardenproject/tracks/internal/track"
)

// Joined is the notice on its main track's relay that side track t
// joined it: its handle, its name and its task's first line.
func Joined(t track.Track) string {
	task, _, _ := strings.Cut(strings.TrimSpace(t.Prompt), "\n")
	return "@" + t.Side + " joined: side track " + t.Name + ", " + strings.TrimSpace(task)
}

// LeftNotice is the notice that side track t left its main track's
// relay, and why: "it ended", "Tracks stopped".
func LeftNotice(t track.Track, why string) string {
	return "@" + t.Side + " left: " + why + "."
}
