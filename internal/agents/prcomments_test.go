package agents

import (
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/commentstyle"
)

// The skill marks what agents post with the marker learning leaves
// out, and points at the guide learning writes.
func TestPRCommentsStepsMatchCommentStyle(t *testing.T) {
	for _, want := range []string{commentstyle.Marker, "tracks/" + commentstyle.FileName} {
		if !strings.Contains(PRCommentsSteps, want) {
			t.Errorf("the steps lack %q", want)
		}
	}
}
