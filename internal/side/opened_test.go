package side

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/golden"
)

func TestOpenedMatchesGolden(t *testing.T) {
	var out strings.Builder
	for _, c := range []struct {
		name        string
		repos, kept []string
	}{
		{"ready", []string{"api"}, nil},
		{"drafts kept", []string{"api"}, []string{"api"}},
		{"some drafts kept", []string{"api", "web"}, []string{"web"}},
		{"no repos from an older daemon", nil, nil},
	} {
		out.WriteString("=== " + c.name + "\n" + Opened("relay-store", "side-000123", c.repos, c.kept) + "\n\n")
	}
	golden.Check(t, filepath.Join("testdata", "opened.golden"), out.String())
}
