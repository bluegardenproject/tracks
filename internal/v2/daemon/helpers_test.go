package daemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bluegardenproject/tracks/internal/v2/settings"
)

func TestHelpersInstallTheCursorRuleOnceCursorIsAdded(t *testing.T) {
	c, _ := config(t)
	c.Paths.Settings = filepath.Join(t.TempDir(), "settings.yaml")
	rule := filepath.Join(c.Home, ".cursor", "rules", "tracks-v2.mdc")

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
	if _, err := os.Stat(filepath.Join(c.Home, ".claude", "skills", "tracks-v2-add-repo", "SKILL.md")); err != nil {
		t.Errorf("the add-repo skill isn't written: %v", err)
	}
}
