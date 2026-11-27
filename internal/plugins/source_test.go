package plugins

import "testing"

func TestParseSource(t *testing.T) {
	tests := []struct {
		arg  string
		want Source
		err  bool
	}{
		{"https://github.com/acme/plugins.git", Source{URL: "https://github.com/acme/plugins.git"}, false},
		{"https://github.com/acme/plugins.git@v1.2", Source{URL: "https://github.com/acme/plugins.git", Ref: "v1.2"}, false},
		{"https://user@github.com/acme/plugins@main", Source{URL: "https://user@github.com/acme/plugins", Ref: "main"}, false},
		{"https://user@host", Source{URL: "https://user@host"}, false},
		{"git@github.com:acme/plugins.git", Source{URL: "git@github.com:acme/plugins.git"}, false},
		{"git@github.com:acme/plugins.git@feature/x", Source{URL: "git@github.com:acme/plugins.git", Ref: "feature/x"}, false},
		{"ssh://git@github.com/acme/p@0123abc", Source{URL: "ssh://git@github.com/acme/p", Ref: "0123abc"}, false},
		{"file:///srv/p@main", Source{URL: "file:///srv/p", Ref: "main"}, false},
		{"/home/u/plugins/acme", Source{Local: "/home/u/plugins/acme"}, false},
		{"./acme", Source{Local: "./acme"}, false},
		{"../acme", Source{Local: "../acme"}, false},
		{"~/acme", Source{Local: "~/acme"}, false},
		{".", Source{Local: "."}, false},
		{"", Source{}, true},
		{"--upload-pack=x", Source{}, true},
		{"https://h/r@-x", Source{}, true},
		{"https://h/r@a..b", Source{}, true},
		{"@main", Source{}, true},
	}
	for _, tt := range tests {
		got, err := ParseSource(tt.arg)
		if (err != nil) != tt.err || got != tt.want {
			t.Errorf("ParseSource(%q) = %+v, %v", tt.arg, got, err)
		}
	}
}
