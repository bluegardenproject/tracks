package plugin

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Bases are the built-in track kinds a track type may extend.
var Bases = []string{"work", "ask", "plan"}

// Reserved names a component can't take: the built-in track kinds and
// the relay's own handles.
var reserved = []string{"work", "ask", "plan", "review", "doc", "reviewer", "track", "tracks", "github", "user"}

// Lifecycles, permissions and the relay kinds a train may send.
var (
	Lifecycles  = []string{"one-shot", "long-running"}
	Permissions = []string{"read", "commands", "write"}
	Sends       = []string{"report", "event", "question", "answer"}
)

// SettingTypes are the types a setting may have.
var SettingTypes = []string{"string", "bool", "int"}

var (
	nameRE    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	settingRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	hostRE    = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]*[a-z0-9])?(:[0-9]{1,5})?$`)
	eventRE   = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	serverRE  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// maxName is the longest plugin or component name.
const maxName = 40

// ValidName reports whether name can be a plugin's or a component's:
// it names folders and relay handles.
func ValidName(name string) bool { return len(name) <= maxName && nameRE.MatchString(name) }

// checker gathers a manifest's problems.
type checker struct {
	problems Problems
	warnings []string
}

func (c *checker) add(field, format string, args ...any) {
	c.problems = append(c.problems, Problem{Field: field, Message: fmt.Sprintf(format, args...)})
}

// Check returns m's warnings and problems.
func Check(m Manifest) ([]string, Problems) {
	c := &checker{}
	c.protocol(m.Protocol)
	c.name("name", m.Name)
	c.engines(m.Engines)
	c.settings(m.Settings)
	c.components(m)
	return c.warnings, c.problems
}

func (c *checker) protocol(v Version) {
	switch {
	case !v.Set:
		c.add("protocol", "is missing; this Tracks speaks protocol %d", ProtocolMajor)
	case v.Major != ProtocolMajor:
		c.add("protocol", "is %s, but this Tracks speaks protocol %d only", v, ProtocolMajor)
	case v.Minor > ProtocolMinor:
		c.warnings = append(c.warnings, fmt.Sprintf(
			"The plugin is written for protocol %s and this Tracks speaks %d.%d: what's newer is ignored. Update Tracks to use all of it.",
			v, ProtocolMajor, ProtocolMinor))
	}
}

func (c *checker) name(field, name string) {
	switch {
	case name == "":
		c.add(field, "is missing")
	case !ValidName(name):
		c.add(field, "%q should be up to %d lowercase letters, digits and dashes, starting with a letter or digit", name, maxName)
	}
}

func (c *checker) engines(engines []string) {
	seen := map[string]bool{}
	for i, e := range engines {
		field := fmt.Sprintf("engines[%d]", i)
		switch {
		case !slices.Contains(Engines, e):
			c.add(field, "%q isn't an engine; use %s", e, orList(Engines))
		case seen[e]:
			c.add(field, "%s is listed twice", e)
		}
		seen[e] = true
	}
}

func (c *checker) settings(settings []Setting) {
	seen := map[string]bool{}
	for i, s := range settings {
		field := fmt.Sprintf("settings[%d]", i)
		switch {
		case s.Name == "":
			c.add(field+".name", "is missing")
		case !settingRE.MatchString(s.Name):
			c.add(field+".name", "%q should be lowercase letters, digits and underscores, starting with a letter", s.Name)
		case seen[s.Name]:
			c.add(field+".name", "another setting is called %s", s.Name)
		}
		seen[s.Name] = true
		if !slices.Contains(SettingTypes, s.Type) {
			c.add(field+".type", "%q isn't a type; use %s", s.Type, orList(SettingTypes))
			continue
		}
		c.values(field, s)
		if s.Default != nil && !defaultFits(s) {
			c.add(field+".default", "%v doesn't fit the setting's type%s", s.Default, valuesHint(s))
		}
	}
}

func (c *checker) values(field string, s Setting) {
	if len(s.Values) == 0 {
		return
	}
	if s.Type != "string" {
		c.add(field+".values", "only string settings can list their values")
		return
	}
	if dup := firstDuplicate(s.Values); dup != "" {
		c.add(field+".values", "%q is listed twice", dup)
	}
}

// defaultFits reports whether s's default is of its type and among its
// values.
func defaultFits(s Setting) bool {
	switch s.Type {
	case "bool":
		_, ok := s.Default.(bool)
		return ok
	case "int":
		_, ok := s.Default.(int)
		return ok
	}
	v, ok := s.Default.(string)
	return ok && (len(s.Values) == 0 || slices.Contains(s.Values, v))
}

func valuesHint(s Setting) string {
	if s.Type == "string" && len(s.Values) > 0 {
		return " or isn't one of " + orList(s.Values)
	}
	return " (" + s.Type + ")"
}

func (c *checker) components(m Manifest) {
	if len(m.Components) == 0 {
		c.add("components", "the plugin brings no components; add a train or a track")
		return
	}
	trains := map[string]bool{}
	for _, comp := range m.Components {
		if comp.Kind == Train {
			trains[comp.Name] = true
		}
	}
	seen := map[string]bool{}
	for i, comp := range m.Components {
		field := fmt.Sprintf("components[%d]", i)
		c.name(field+".name", comp.Name)
		switch {
		case slices.Contains(reserved, comp.Name):
			c.add(field+".name", "%s is Tracks' own; pick another name", comp.Name)
		case comp.Name != "" && seen[comp.Name]:
			c.add(field+".name", "another component is called %s", comp.Name)
		}
		seen[comp.Name] = true
		c.path(field+".brief", comp.Brief)
		c.paths(field+".context", comp.Context)
		switch comp.Kind {
		case Track:
			c.track(field, comp, trains, m.Engines)
		case Train:
			c.train(field, comp)
		case "":
			c.add(field+".kind", "is missing; use train or track")
		default:
			c.add(field+".kind", "%q isn't a kind; use train or track", comp.Kind)
		}
	}
}

func (c *checker) track(field string, comp Component, trains map[string]bool, engines []string) {
	switch {
	case comp.Base == "":
		c.add(field+".base", "is missing; a track type extends %s", orList(Bases))
	case comp.Base == "review" || comp.Base == "doc":
		c.add(field+".base", "%s tracks can't be extended yet; use %s", comp.Base, orList(Bases))
	case !slices.Contains(Bases, comp.Base):
		c.add(field+".base", "%q isn't a track kind; use %s", comp.Base, orList(Bases))
	}
	for i, t := range comp.Trains {
		if t != "reviewer" && !trains[t] {
			c.add(fmt.Sprintf("%s.trains[%d]", field, i), "%q is neither reviewer nor a train of this plugin", t)
		}
	}
	for i, a := range comp.Actions {
		if strings.TrimSpace(a) == "" {
			c.add(fmt.Sprintf("%s.actions[%d]", field, i), "is empty")
		}
	}
	if d := comp.Defaults; d != nil && d.Engine != "" {
		allowed := engines
		if len(allowed) == 0 {
			allowed = Engines
		}
		if !slices.Contains(allowed, d.Engine) {
			c.add(field+".defaults.engine", "%q isn't one of the plugin's engines (%s)", d.Engine, orList(allowed))
		}
	}
	for _, f := range trainFields(comp) {
		c.add(field+"."+f, "is for train components only")
	}
}

func (c *checker) train(field string, comp Component) {
	c.oneOf(field+".lifecycle", comp.Lifecycle, Lifecycles)
	c.oneOf(field+".permissions", comp.Permissions, Permissions)
	for i, h := range comp.Hosts {
		if !hostRE.MatchString(h) {
			c.add(fmt.Sprintf("%s.hosts[%d]", field, i), "%q isn't a host name such as api.example.com or *.example.com", h)
		}
	}
	c.paths(field+".mcp", comp.MCP)
	for i, s := range comp.RequiresMCP {
		if !serverRE.MatchString(s) {
			c.add(fmt.Sprintf("%s.requires_mcp[%d]", field, i), "%q isn't an MCP server's name", s)
		}
	}
	if r := comp.Relay; r != nil {
		for i, k := range r.Sends {
			f := fmt.Sprintf("%s.relay.sends[%d]", field, i)
			switch {
			case k == "notice":
				c.add(f, "notices are Tracks' own; use %s", orList(Sends))
			case !slices.Contains(Sends, k):
				c.add(f, "%q isn't a kind of message; use %s", k, orList(Sends))
			}
		}
		for i, e := range r.Subscribes {
			if !eventRE.MatchString(e) {
				c.add(fmt.Sprintf("%s.relay.subscribes[%d]", field, i), "%q isn't an event such as github.ci_failed", e)
			}
		}
	}
	for _, f := range trackFields(comp) {
		c.add(field+"."+f, "is for track components only")
	}
}

func (c *checker) oneOf(field, v string, allowed []string) {
	switch {
	case v == "":
		c.add(field, "is missing; use %s", orList(allowed))
	case !slices.Contains(allowed, v):
		c.add(field, "%q isn't allowed; use %s", v, orList(allowed))
	}
}

func (c *checker) paths(field string, ps []string) {
	for i, p := range ps {
		c.path(fmt.Sprintf("%s[%d]", field, i), p)
	}
}

// path checks that p names something inside the plugin's folder.
func (c *checker) path(field, p string) {
	switch {
	case p == "":
		c.add(field, "is missing")
	case !Inside(p):
		c.add(field, "%q should be a path inside the plugin's folder, such as briefs/main.md", p)
	}
}

// Inside reports whether p, a manifest's path, stays inside the
// plugin's folder: relative, without .. leading out, and not the
// folder itself.
func Inside(p string) bool {
	if strings.Contains(p, "\\") || strings.ContainsRune(p, 0) || path.IsAbs(p) {
		return false
	}
	clean := path.Clean(p)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func trainFields(c Component) []string {
	var set []string
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"lifecycle", c.Lifecycle != ""},
		{"permissions", c.Permissions != ""},
		{"hosts", len(c.Hosts) > 0},
		{"mcp", len(c.MCP) > 0},
		{"requires_mcp", len(c.RequiresMCP) > 0},
		{"relay", c.Relay != nil},
	} {
		if f.set {
			set = append(set, f.name)
		}
	}
	return set
}

func trackFields(c Component) []string {
	var set []string
	for _, f := range []struct {
		name string
		set  bool
	}{
		{"base", c.Base != ""},
		{"trains", len(c.Trains) > 0},
		{"actions", len(c.Actions) > 0},
		{"defaults", c.Defaults != nil},
	} {
		if f.set {
			set = append(set, f.name)
		}
	}
	return set
}

func firstDuplicate(vs []string) string {
	seen := map[string]bool{}
	for _, v := range vs {
		if seen[v] {
			return v
		}
		seen[v] = true
	}
	return ""
}

// orList joins vs as "a, b or c".
func orList(vs []string) string {
	if len(vs) < 2 {
		return strings.Join(vs, "")
	}
	return strings.Join(vs[:len(vs)-1], ", ") + " or " + vs[len(vs)-1]
}
