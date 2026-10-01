package footer

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/theme"
)

func TestSlotsMarkTracksThatNeedTheUser(t *testing.T) {
	th := theme.Default()
	row := nav(palette{theme: th})
	mark := "#{?@tracks_attention,#[fg=" + th.Value(theme.StateWarningText) + "]● "
	if strings.Count(row, mark) != 2 {
		t.Errorf("both the current and the other slots should carry the mark:\n%s", row)
	}
}
