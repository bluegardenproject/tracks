package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var notifyKeys = []string{NotifyMacOS, NotifyBell, NotifyActionRequired, NotifyError, NotifyPROpened, NotifyPRSettled}

func TestNotificationsDefaultOn(t *testing.T) {
	var n Notifications
	for _, k := range notifyKeys {
		if !n.On(k) {
			t.Errorf("%s is off by default", k)
		}
	}
	if n.On("agent_exited") {
		t.Error("an unknown key is on")
	}
}

func TestNotificationsSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(path, []byte("notifications:\n  bell: false\n  sound: glass # from a newer build\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || s.Notifications.On(NotifyBell) || !s.Notifications.On(NotifyMacOS) {
		t.Fatalf("Load: %+v, %v", s.Notifications, err)
	}

	s.Notifications = s.Notifications.Set(NotifyBell, true).Set(NotifyPROpened, false)
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if got := string(data); strings.Contains(got, "bell") || !strings.Contains(got, "pr_opened: false") || !strings.Contains(got, "sound: glass") {
		t.Errorf("saved:\n%s\nwant bell gone, pr_opened: false and the unknown key kept", got)
	}
	s, _ = Load(path)
	for _, k := range notifyKeys {
		if want := k != NotifyPROpened; s.Notifications.On(k) != want {
			t.Errorf("after Save, %s on = %v, want %v", k, !want, want)
		}
	}
}
