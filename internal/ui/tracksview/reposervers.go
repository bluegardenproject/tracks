package tracksview

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"github.com/bluegardenproject/tracks/internal/ui/source"
	"github.com/bluegardenproject/tracks/internal/ui/widget"
)

// A dev server's fields, in display order. The ones before serverMode
// are text inputs, as are serverPort and serverType.
const (
	serverName = iota
	serverCommand
	serverDir
	serverMode
	serverPort
	serverType
	serverRemove
	serverFieldCount
)

// serverInputs are a dev server's text fields, indexed by field.
var serverInputs = map[int]struct {
	label, desc, key string
	limit            int
	placeholder      string
}{
	serverName:    {"Name", "Shown in the Proxy tab's list of servers.", "name", 32, "web"},
	serverCommand: {"Command", "Starts the server. $PORT holds its port when Tracks assigns one.", "command", 4096, "pnpm dev --port $PORT"},
	serverDir:     {"Directory", "Where the command runs, inside the repo; its root when left empty.", "dir", 4096, "apps/web"},
	serverPort:    {"Port number", "The port the server always uses.", "port", 5, "8081"},
	serverType:    {"Type", "A label such as rspack, metro or vite, to tell servers apart. Optional.", "type", 32, "rspack"},
}

// portModes are the port modes in the order Space cycles through them,
// with what each means.
var portModes = []struct{ mode, desc string }{
	{source.PortAssigned, "Tracks passes a free port as $PORT."},
	{source.PortFixed, "The server always uses one port."},
	{source.PortDetect, "Tracks reads the port the server listens on."},
}

// serverForm edits one dev server of the repo form.
type serverForm struct {
	inputs [serverFieldCount]textinput.Model // only the text fields are used
	mode   string
}

func newServerForm(d source.DevServer) serverForm {
	s := serverForm{mode: d.PortMode}
	if s.mode == "" {
		s.mode = source.PortAssigned
	}
	for k, in := range serverInputs {
		s.inputs[k] = widget.NewInput(in.limit, in.placeholder)
	}
	s.inputs[serverName].SetValue(d.Name)
	s.inputs[serverCommand].SetValue(d.Command)
	s.inputs[serverDir].SetValue(d.Dir)
	s.inputs[serverType].SetValue(d.Type)
	if d.Port != 0 {
		s.inputs[serverPort].SetValue(strconv.Itoa(d.Port))
	}
	return s
}

func (s serverForm) value(k int) string { return strings.TrimSpace(s.inputs[k].Value()) }

// server is what the form would save. A port that isn't a number is
// saved as -1, which the repo rules reject.
func (s serverForm) server() source.DevServer {
	d := source.DevServer{Name: s.value(serverName), Command: s.value(serverCommand), Dir: s.value(serverDir),
		PortMode: s.mode, Type: s.value(serverType)}
	if s.mode == source.PortFixed {
		port, err := strconv.Atoi(s.value(serverPort))
		if err != nil {
			port = -1
		}
		d.Port = port
	}
	return d
}

// fields are the server's fields shown, in display order.
func (s serverForm) fields() []int {
	if s.mode == source.PortFixed {
		return []int{serverName, serverCommand, serverDir, serverMode, serverPort, serverType, serverRemove}
	}
	return []int{serverName, serverCommand, serverDir, serverMode, serverType, serverRemove}
}

func (s *serverForm) input(k int) *textinput.Model {
	if _, ok := serverInputs[k]; !ok {
		return nil
	}
	return &s.inputs[k]
}

// nextMode moves the port mode on by one, wrapping.
func (s *serverForm) nextMode() {
	for i, m := range portModes {
		if m.mode == s.mode {
			s.mode = portModes[(i+1)%len(portModes)].mode
			return
		}
	}
	s.mode = source.PortAssigned
}

// serverField is dev server i's field k.
func serverField(i, k int) formField { return fieldServers + formField(i*serverFieldCount+k) }

// server returns the dev server and its field that f is, if it's one.
func (f formField) server() (i, k int, ok bool) {
	if f < fieldServers {
		return 0, 0, false
	}
	n := int(f - fieldServers)
	return n / serverFieldCount, n % serverFieldCount, true
}

// serverErrKey is the key dev server i's field k's errors come under.
func serverErrKey(i, k int) string {
	switch k {
	case serverMode:
		k = serverPort
	case serverRemove:
		return ""
	}
	return source.ServerField(i, serverInputs[k].key)
}
