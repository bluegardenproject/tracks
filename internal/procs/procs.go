// Package procs reads the system's processes and the TCP ports they
// listen on, for finding the dev servers running in a track's panes.
package procs

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Proc is a process.
type Proc struct {
	PID, PPID int
	Command   string // its command line
}

// Snapshot is the processes and their listening ports at one moment.
type Snapshot struct {
	Procs map[int]Proc
	// Ports are the TCP ports each process listens on, sorted.
	Ports    map[int][]int
	children map[int][]int
}

// New builds a snapshot from processes and their ports.
func New(procs map[int]Proc, ports map[int][]int) Snapshot {
	s := Snapshot{Procs: procs, Ports: ports, children: map[int][]int{}}
	for _, p := range procs {
		s.children[p.PPID] = append(s.children[p.PPID], p.PID)
	}
	for _, c := range s.children {
		sort.Ints(c)
	}
	return s
}

// Take reads the processes with ps and the listening ports with lsof.
func Take(ctx context.Context) (Snapshot, error) {
	// -ww: Kind reads whole command lines, which ps cuts to the
	// terminal's width otherwise.
	ps, err := exec.CommandContext(ctx, "ps", "-A", "-ww", "-o", "pid=,ppid=,command=").Output()
	if err != nil {
		return Snapshot{}, err
	}
	lsof, err := exec.CommandContext(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-F", "pn").Output()
	// lsof exits 1 when nothing listens.
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return Snapshot{}, err
	}
	return New(ParsePS(string(ps)), ParseLsof(string(lsof))), nil
}

// ParsePS reads `ps -o pid=,ppid=,command=` output.
func ParsePS(out string) map[int]Proc {
	procs := map[int]Proc{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			continue
		}
		procs[pid] = Proc{PID: pid, PPID: ppid, Command: strings.Join(f[2:], " ")}
	}
	return procs
}

// ParseLsof reads `lsof -F pn` output: a p line per process, then an n
// line per address it listens on, such as 127.0.0.1:5173 or [::1]:3000.
func ParseLsof(out string) map[int][]int {
	ports := map[int][]int{}
	pid := 0
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 'n':
			i := strings.LastIndexByte(line, ':')
			if i < 0 || pid == 0 {
				continue
			}
			port, err := strconv.Atoi(line[i+1:])
			if err != nil {
				continue
			}
			if !containsInt(ports[pid], port) {
				ports[pid] = append(ports[pid], port)
				sort.Ints(ports[pid])
			}
		}
	}
	return ports
}

// Listener is a process below another one that listens on a port.
type Listener struct {
	PID, Port int
	Depth     int // 1 for a child of the process it's below
}

// Below lists the ports the processes below pid listen on, pid itself
// at depth 0, by depth then port.
func (s Snapshot) Below(pid int) []Listener {
	var out []Listener
	var walk func(pid, depth int)
	walk = func(pid, depth int) {
		for _, port := range s.Ports[pid] {
			out = append(out, Listener{PID: pid, Port: port, Depth: depth})
		}
		for _, c := range s.children[pid] {
			walk(c, depth+1)
		}
	}
	if _, ok := s.Procs[pid]; ok {
		walk(pid, 0)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Depth != out[j].Depth {
			return out[i].Depth < out[j].Depth
		}
		return out[i].Port < out[j].Port
	})
	return out
}

// kinds are the tools Kind knows, by the word that gives them away.
var kinds = []struct{ word, kind string }{
	{"metro", "metro"}, {"rspack", "rspack"}, {"rsbuild", "rsbuild"}, {"vite", "vite"}, {"webpack", "webpack"},
	{"next", "next"}, {"storybook", "storybook"}, {"electron", "electron"}, {"astro", "astro"}, {"nuxt", "nuxt"},
	{"remix", "remix"},
}

// Kind guesses what tool a command line runs: a known dev server, or
// else the program's name, such as node. Only the last part of each
// path counts, so a folder called next doesn't make a Next server.
func Kind(command string) string {
	var words []string
	for _, f := range strings.Fields(strings.ToLower(command)) {
		words = append(words, strings.Split(filepath.Base(f), "@")...)
	}
	for i, w := range words {
		if w == "react-native" && i+1 < len(words) && words[i+1] == "start" {
			return "metro"
		}
	}
	for _, k := range kinds {
		for _, w := range words {
			if w == k.word || strings.HasPrefix(w, k.word+"-") || strings.HasPrefix(w, k.word+".") {
				return k.kind
			}
		}
	}
	return Program(command)
}

// Program is the name of the program a command line runs: its first
// word's last part. A path with spaces in it is cut at the first one.
func Program(command string) string {
	first, _, _ := strings.Cut(command, " ")
	return strings.TrimPrefix(filepath.Base(first), "-") // a login shell is -zsh
}

// shells are the programs a command runs through.
var shells = map[string]bool{"sh": true, "bash": true, "zsh": true, "dash": true, "fish": true, "ksh": true}

// IsShell reports whether command runs a shell.
func IsShell(command string) bool { return shells[Program(command)] }

// Runs reports whether command runs program: as its first word, or as
// the script an interpreter such as node runs.
func Runs(command, program string) bool {
	f := strings.Fields(command)
	if len(f) > 0 && Program(f[0]) == program {
		return true
	}
	return len(f) > 1 && filepath.Base(f[1]) == program
}

// Parent is pid's parent, 0 for none known.
func (s Snapshot) Parent(pid int) int { return s.Procs[pid].PPID }

// Children are pid's children.
func (s Snapshot) Children(pid int) []int { return s.children[pid] }

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
