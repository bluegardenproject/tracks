package platform

import (
	"path/filepath"
	"strings"
	"testing"

	v1config "github.com/bluegardenproject/tracks/internal/config"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name string
		env  env
		want Paths
	}{
		{"default", env{home: "/home/u"}, Paths{
			ConfigDir:  "/home/u/.config/tracks-v2",
			Settings:   "/home/u/.config/tracks-v2/settings.yaml",
			ThemesDir:  "/home/u/.config/tracks-v2/themes",
			DataDir:    "/home/u/.local/state/tracks-v2",
			Database:   "/home/u/.local/state/tracks-v2/tracks.db",
			Worktrees:  "/home/u/.local/state/tracks-v2/worktrees",
			Socket:     "/home/u/.local/state/tracks-v2/daemon.sock",
			Lock:       "/home/u/.local/state/tracks-v2/daemon.lock",
			Log:        "/home/u/.local/state/tracks-v2/daemon.log",
			BinDir:     "/home/u/.local/state/tracks-v2/bin",
			TmuxSocket: "tracks-v2",
		}},
		{"XDG dirs", env{home: "/home/u", xdgConfig: "/xc", xdgState: "/xs"}, Paths{
			ConfigDir:  "/xc/tracks-v2",
			Settings:   "/xc/tracks-v2/settings.yaml",
			ThemesDir:  "/xc/tracks-v2/themes",
			DataDir:    "/xs/tracks-v2",
			Database:   "/xs/tracks-v2/tracks.db",
			Worktrees:  "/xs/tracks-v2/worktrees",
			Socket:     "/xs/tracks-v2/daemon.sock",
			Lock:       "/xs/tracks-v2/daemon.lock",
			Log:        "/xs/tracks-v2/daemon.log",
			BinDir:     "/xs/tracks-v2/bin",
			TmuxSocket: "tracks-v2",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolve(tt.env); got != tt.want {
				t.Errorf("resolve() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Both apps run side by side, so no v2 location may sit inside v1's.
func TestPathsStayOutOfV1(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	v1ConfigFile, err := v1config.Path()
	if err != nil {
		t.Fatal(err)
	}
	v1StateDir, err := v1config.Default().ResolveStateDir()
	if err != nil {
		t.Fatal(err)
	}
	v1Dirs := []string{filepath.Dir(v1ConfigFile), v1StateDir}

	p, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	for _, v2 := range []string{p.ConfigDir, p.DataDir} {
		for _, v1 := range v1Dirs {
			if v2 == v1 || strings.HasPrefix(v2, v1+string(filepath.Separator)) {
				t.Errorf("%s is inside v1's %s", v2, v1)
			}
		}
	}
	if p.TmuxSocket == "default" || p.TmuxSocket == "" {
		t.Errorf("tmux socket %q is the default server", p.TmuxSocket)
	}
}
