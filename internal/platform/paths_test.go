package platform

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name string
		env  env
		want Paths
	}{
		{"default", env{home: "/home/u"}, Paths{
			ConfigDir:  "/home/u/.config/tracks",
			Settings:   "/home/u/.config/tracks/settings.yaml",
			ThemesDir:  "/home/u/.config/tracks/themes",
			DataDir:    "/home/u/.local/state/tracks",
			Database:   "/home/u/.local/state/tracks/tracks.db",
			Worktrees:  "/home/u/.local/state/tracks/worktrees",
			Socket:     "/home/u/.local/state/tracks/daemon.sock",
			Lock:       "/home/u/.local/state/tracks/daemon.lock",
			Log:        "/home/u/.local/state/tracks/daemon.log",
			BinDir:     "/home/u/.local/state/tracks/bin",
			TmuxSocket: "tracks",
		}},
		{"XDG dirs", env{home: "/home/u", xdgConfig: "/xc", xdgState: "/xs"}, Paths{
			ConfigDir:  "/xc/tracks",
			Settings:   "/xc/tracks/settings.yaml",
			ThemesDir:  "/xc/tracks/themes",
			DataDir:    "/xs/tracks",
			Database:   "/xs/tracks/tracks.db",
			Worktrees:  "/xs/tracks/worktrees",
			Socket:     "/xs/tracks/daemon.sock",
			Lock:       "/xs/tracks/daemon.lock",
			Log:        "/xs/tracks/daemon.log",
			BinDir:     "/xs/tracks/bin",
			TmuxSocket: "tracks",
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
