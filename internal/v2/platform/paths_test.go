package platform

import (
	"path/filepath"
	"strings"
	"testing"

	v1config "github.com/bluegardenproject/tracks/internal/config"
)

func TestResolve(t *testing.T) {
	base := env{home: "/home/u", tempDir: "/tmp", uid: 501}
	tests := []struct {
		name    string
		profile Profile
		env     env
		want    Paths
	}{
		{"default", Default, base, Paths{
			ConfigDir:  "/home/u/.config/tracks-v2",
			DataDir:    "/home/u/.local/state/tracks-v2",
			Database:   "/home/u/.local/state/tracks-v2/tracks.db",
			TmuxSocket: "tracks-v2",
		}},
		{"XDG dirs", Default, env{home: "/home/u", xdgConfig: "/xc", xdgState: "/xs"}, Paths{
			ConfigDir:  "/xc/tracks-v2",
			DataDir:    "/xs/tracks-v2",
			Database:   "/xs/tracks-v2/tracks.db",
			TmuxSocket: "tracks-v2",
		}},
		{"demo", Demo, base, Paths{
			ConfigDir:  "/home/u/.config/tracks-v2",
			DataDir:    "/tmp/tracks-v2-demo-501",
			Database:   "/home/u/.local/state/tracks-v2/tracks.db",
			TmuxSocket: "tracks-v2-demo",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolve(tt.profile, tt.env); got != tt.want {
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

	for _, profile := range []Profile{Default, Demo} {
		p, err := Resolve(profile)
		if err != nil {
			t.Fatal(err)
		}
		for _, v2 := range []string{p.ConfigDir, p.DataDir} {
			for _, v1 := range v1Dirs {
				if v2 == v1 || strings.HasPrefix(v2, v1+string(filepath.Separator)) {
					t.Errorf("profile %d: %s is inside v1's %s", profile, v2, v1)
				}
			}
		}
		if p.TmuxSocket == "default" || p.TmuxSocket == "" {
			t.Errorf("profile %d: tmux socket %q is the default server", profile, p.TmuxSocket)
		}
	}
}
