package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/settings"
)

func TestHelpersInstallTheCursorRuleOnceCursorIsAdded(t *testing.T) {
	c, _ := config(t)
	c.Paths.Settings = filepath.Join(t.TempDir(), "settings.yaml")
	rule := filepath.Join(c.Home, ".cursor", "rules", "tracks.mdc")

	c.helpers()
	if _, err := os.Stat(rule); !os.IsNotExist(err) {
		t.Fatalf("the rule is written without Cursor: %v", err)
	}
	if err := settings.Save(c.Paths.Settings, settings.Settings{Engines: settings.Engines{Cursor: &settings.Engine{}}}); err != nil {
		t.Fatal(err)
	}
	c.helpers()
	if _, err := os.Stat(rule); err != nil {
		t.Errorf("the rule isn't written with Cursor added: %v", err)
	}
	for _, path := range []string{
		filepath.Join(c.Home, ".cursor", "rules", "tracks-pr-comments.mdc"),
		filepath.Join(c.Home, ".claude", "skills", "tracks-add-repo", "SKILL.md"),
		filepath.Join(c.Home, ".claude", "skills", "tracks-pr-comments", "SKILL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s isn't written: %v", filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), err)
		}
	}
}
