package procs

import (
	"context"
	"net"
	"os"
	"os/exec"
	"slices"
	"testing"
)

func TestParseAndBelow(t *testing.T) {
	ps := `  1     0 /sbin/launchd
 100     1 tmux -L tracks new-session
 200   100 sh -c claude go
 201   200 claude go
 202   201 node mcp-server.js
 203   201 /bin/zsh -c pnpm dev
 204   203 node /src/web/node_modules/.bin/vite --port 5173
 300   100 sh -c bash
garbage line
`
	lsof := "p201\nn127.0.0.1:55367\np202\nn127.0.0.1:3845\np204\nn[::1]:5173\nn127.0.0.1:5173\np999\nn*:7000\n"
	s := New(ParsePS(ps), ParseLsof(lsof))
	if got := s.Ports[204]; !slices.Equal(got, []int{5173}) {
		t.Errorf("vite's ports = %v; want 5173 once for both loopbacks", got)
	}
	want := []Listener{{201, 55367, 1}, {202, 3845, 2}, {204, 5173, 3}}
	if got := s.Below(200); !slices.Equal(got, want) {
		t.Errorf("Below(200) = %+v, want %+v", got, want)
	}
	if got := s.Below(300); len(got) != 0 {
		t.Errorf("Below(300) = %+v, want none", got)
	}
	if got := s.Below(12345); got != nil {
		t.Errorf("Below of an unknown process = %+v", got)
	}
}

func TestKind(t *testing.T) {
	for command, want := range map[string]string{
		"node /src/node_modules/.bin/../.pnpm/vite@8.3.2_@types+node@24/node_modules/vite/bin/vite.js": "vite",
		"node /src/node_modules/@rspack/cli/bin/rspack.js serve":                                       "rspack",
		"node /src/node_modules/.bin/react-native start --port 8081":                                   "metro",
		"node /src/node_modules/next/dist/bin/next dev":                                                "next",
		"/Applications/Electron.app/Contents/MacOS/Electron .":                                         "electron",
		"python3 -m http.server 8000":                                                                  "python3",
		"/usr/local/bin/node server.js":                                                                "node",
		"node /src/nextcloud/server.js":                                                                "node",
		"node /src/next/server.js":                                                                     "node",
		"/src/remix-app/node_modules/.bin/vite":                                                        "vite",
	} {
		if got := Kind(command); got != want {
			t.Errorf("Kind(%q) = %q, want %q", command, got, want)
		}
	}
}

// TestTake finds a real listener of this test process.
func TestTake(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	for _, tool := range []string{"ps", "lsof"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s: %v", tool, err)
		}
	}
	s, err := Take(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if !slices.Contains(s.Ports[os.Getpid()], port) {
		t.Errorf("this process's ports = %v; want %d", s.Ports[os.Getpid()], port)
	}
}

func TestShellsAndPrograms(t *testing.T) {
	for command, want := range map[string]bool{"/bin/zsh -c pnpm dev": true, "-zsh": true, "sh": true, "node x.js": false, "zshx": false} {
		if got := IsShell(command); got != want {
			t.Errorf("IsShell(%q) = %v", command, got)
		}
	}
	for _, c := range []struct {
		command, program string
		want             bool
	}{
		{"claude go", "claude", true},
		{"/usr/local/bin/claude --resume x", "claude", true},
		{"node /opt/homebrew/lib/node_modules/@anthropic-ai/claude-code/cli.js", "claude", false},
		{"node /home/u/.local/bin/agent --force", "agent", true},
		{"pnpm dev", "claude", false},
	} {
		if got := Runs(c.command, c.program); got != c.want {
			t.Errorf("Runs(%q, %q) = %v", c.command, c.program, got)
		}
	}
}
