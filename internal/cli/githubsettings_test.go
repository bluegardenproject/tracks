package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluegardenproject/tracks/internal/settings"
)

func TestGitHubSettingsKeepTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	before := "theme: default_light\nnotifications:\n  bell: false\ngithub:\n  ci_failed: sometimes\n  stale: ask # from a newer build\n"
	if err := os.WriteFile(path, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	src := githubSettings{path: path}
	g, err := src.Load()
	if err != nil || g.Policy(settings.EventCIFailed) != settings.PolicyAuto {
		t.Fatalf("Load = %+v, %v; want the unknown value read as the default", g, err)
	}
	g.Conflicts = settings.PolicyAsk
	if err := src.Save(g); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, want := range []string{"theme: default_light", "bell: false", "conflicts: ask", "ci_failed: sometimes", "stale: ask # from a newer build"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("saved file lacks %q:\n%s", want, data)
		}
	}
}
