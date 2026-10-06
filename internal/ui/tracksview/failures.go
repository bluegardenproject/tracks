package tracksview

import (
	"fmt"

	"github.com/bluegardenproject/tracks/internal/theme"
	"github.com/bluegardenproject/tracks/internal/track"
	"github.com/bluegardenproject/tracks/internal/ui/source"
)

// dismissAction clears a track's server and setup errors.
var dismissAction = action{actionDismissFailures, "Dismiss track errors", "t", 8}

// failureLine says what failed, as the details list it.
func failureLine(f track.Failure) string {
	if f.Kind == track.SetupError {
		return fmt.Sprintf("setup of %s failed (exit %d)", f.Subject, f.Code)
	}
	return fmt.Sprintf("server %s crashed (exit %d)", f.Subject, f.Code)
}

// failureLines appends t's errors to the details' lines, under label.
func (m Model) failureLines(lines []string, t source.Track, label func(string) string, width int) []string {
	for i, f := range t.Failures {
		l := label("")
		if i == 0 {
			l = label("Errors")
		}
		lines = append(lines, l+m.fg(theme.StateDangerText).Render(cut(failureLine(f), max(0, width-labelWidth))))
	}
	return lines
}

// dismissRow appends Dismiss track errors while t has errors and no
// question is open: on a row of its own, a line below the others.
func (m Model) dismissRow(lines []string, hits []hit, t source.Track, width int) ([]string, []hit) {
	if len(t.Failures) == 0 || m.station.asking != nil {
		return lines, hits
	}
	row, more := m.buttonRows([]action{dismissAction}, t, width, len(lines)+1)
	return append(append(lines, ""), row...), append(hits, more...)
}
