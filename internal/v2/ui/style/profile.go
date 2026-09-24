package style

import (
	"os"

	"github.com/charmbracelet/colorprofile"
)

// Profile is the colour profile for Tracks screens: 24-bit whenever
// colours are allowed at all (NO_COLOR is respected). Inside Tracks'
// tmux server that's always right, because tmux converts to what each
// attached terminal supports. Detection alone would settle on 256
// colours: inside tmux it asks the attached terminal, and window 0
// starts before any terminal attaches.
func Profile() colorprofile.Profile {
	if p := colorprofile.Detect(os.Stdout, os.Environ()); p < colorprofile.ANSI {
		return p
	}
	return colorprofile.TrueColor
}
