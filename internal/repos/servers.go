package repos

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/bluegardenproject/tracks/internal/store"
	"github.com/bluegardenproject/tracks/internal/track"
)

// The longest dev server name and type, in characters.
const (
	maxServerName = 32
	maxServerType = 32
	maxCommand    = 4096
)

// ServerField is the FieldError field of dev server i's field: name,
// command, dir, port or type.
func ServerField(i int, field string) string { return fmt.Sprintf("servers.%d.%s", i, field) }

// checkServers normalizes r's setup and dev servers and reports the
// first problem.
func checkServers(r store.Repo) (store.Repo, error) {
	r.Setup = strings.TrimSpace(r.Setup)
	if r.Setup != "" && !printable(r.Setup, maxCommand) {
		return r, &FieldError{"setup", "Enter the setup command on one line; chain steps with &&."}
	}
	if len(r.Servers) == 0 {
		r.Servers = nil
		return r, nil
	}
	servers := make([]store.DevServer, len(r.Servers))
	names, ports := map[string]bool{}, map[int]bool{}
	for i, d := range r.Servers {
		d, err := checkServer(i, d)
		if err != nil {
			return r, err
		}
		key := strings.ToLower(d.Name)
		if names[key] {
			return r, &FieldError{ServerField(i, "name"), "Another dev server of this repo has this name."}
		}
		names[key] = true
		if d.PortMode == store.PortFixed {
			if ports[d.Port] {
				return r, &FieldError{ServerField(i, "port"), "Another dev server of this repo uses this port."}
			}
			ports[d.Port] = true
		}
		servers[i] = d
	}
	r.Servers = servers
	return r, nil
}

func checkServer(i int, d store.DevServer) (store.DevServer, error) {
	d.Name, d.Command, d.Type = strings.TrimSpace(d.Name), strings.TrimSpace(d.Command), strings.TrimSpace(d.Type)
	d.Dir = strings.TrimSpace(d.Dir)
	if !printable(d.Name, maxServerName) {
		return d, &FieldError{ServerField(i, "name"), fmt.Sprintf("Enter a name of up to %d characters.", maxServerName)}
	}
	if !printable(d.Command, maxCommand) {
		return d, &FieldError{ServerField(i, "command"), "Enter the command that starts the server, on one line."}
	}
	if d.Dir != "" {
		clean := filepath.Clean(d.Dir)
		if !printable(clean, maxCommand) || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return d, &FieldError{ServerField(i, "dir"), "Use a folder inside the repo, such as apps/web."}
		}
		d.Dir = clean
		if clean == "." {
			d.Dir = ""
		}
	}
	switch d.PortMode {
	case "":
		d.PortMode = store.PortAssigned
		fallthrough
	case store.PortAssigned, store.PortDetect:
		d.Port = 0
	case store.PortFixed:
		if d.Port < 1024 || d.Port > 65535 || (d.Port >= track.FirstAssignedPort && d.Port <= track.LastAssignedPort) {
			return d, &FieldError{ServerField(i, "port"), fmt.Sprintf("Enter a port from 1024 to 65535, outside %d–%d, which Tracks assigns.",
				track.FirstAssignedPort, track.LastAssignedPort)}
		}
	default:
		return d, &FieldError{ServerField(i, "port"), "Pick assigned, fixed or detect."}
	}
	if d.Type != "" && !printable(d.Type, maxServerType) {
		return d, &FieldError{ServerField(i, "type"), fmt.Sprintf("Enter a type of up to %d characters.", maxServerType)}
	}
	return d, nil
}

// printable reports whether s is 1 to limit characters of text without
// control characters.
func printable(s string, limit int) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= limit && !strings.ContainsFunc(s, unicode.IsControl)
}
