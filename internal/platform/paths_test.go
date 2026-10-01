package platform

import "testing"

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
