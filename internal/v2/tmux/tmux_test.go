package tmux

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestParseVersion(t *testing.T) {
	tests := map[string]Version{
		"tmux 3.5a":      {3, 5},
		"tmux 3.2":       {3, 2},
		"tmux next-3.6":  {3, 6},
		"tmux 3.3-rc":    {3, 3},
		"tmux master":    {},
		"tmux openbsd-x": {},
	}
	for in, want := range tests {
		if got := ParseVersion(in); got != want {
			t.Errorf("ParseVersion(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		v, want Version
		ok      bool
	}{
		{Version{3, 2}, MinVersion, true},
		{Version{3, 1}, MinVersion, false},
		{Version{2, 9}, MinVersion, false},
		{Version{4, 0}, MinVersion, true},
		{Version{}, MinVersion, true},
	}
	for _, tt := range tests {
		if got := tt.v.AtLeast(tt.want); got != tt.ok {
			t.Errorf("%v.AtLeast(%v) = %v, want %v", tt.v, tt.want, got, tt.ok)
		}
	}
}

func TestLocationOf(t *testing.T) {
	tests := []struct {
		env  string
		want Location
	}{
		{"", Outside},
		{"/private/tmp/tmux-501/tracks-v2,123,0", OwnServer},
		{"/private/tmp/tmux-501/default,123,2", OtherServer},
		{"/private/tmp/tmux-501/tracks-v2-demo,123,0", OtherServer},
	}
	for _, tt := range tests {
		if got := LocationOf(tt.env, "tracks-v2"); got != tt.want {
			t.Errorf("LocationOf(%q) = %v, want %v", tt.env, got, tt.want)
		}
	}
}

func TestArgsAddSocket(t *testing.T) {
	got := New(TestSocketPrefix + "x").args("list-sessions")
	want := []string{"-L", TestSocketPrefix + "x", "list-sessions"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %q, want %q", got, want)
	}
}

func TestRealSocketPanicsInTests(t *testing.T) {
	for _, socket := range []string{"tracks-v2", "default"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("socket %q: no panic", socket)
				}
			}()
			New(socket).args("list-sessions")
		}()
	}
}

func TestConfRender(t *testing.T) {
	confs := map[string]Conf{
		"tmux-3.2":           {Version: Version{3, 2}, OuterTerm: "xterm-256color"},
		"tmux-3.4-truecolor": {Version: Version{3, 4}, OuterTerm: "xterm-ghostty", TrueColor: true},
	}
	for name, conf := range confs {
		t.Run(name, func(t *testing.T) {
			conf.DefaultTerminal = "tmux-256color"
			conf.OverrideFile = "/home/u/.config/tracks-v2/tmux.conf"
			got, err := conf.Render()
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join("testdata", name+".conf")
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Errorf("config differs from %s (run with -update to accept):\n%s", golden, got)
			}
		})
	}
}
