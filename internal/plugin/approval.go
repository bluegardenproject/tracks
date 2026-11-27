package plugin

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// Approval is what the user agrees to when installing a plugin: its
// components with their rights, and the code Tracks will run. It is
// kept, so an update can tell whether it asks for more.
type Approval struct {
	Components []Grant `json:"components"`
	Code       []Code  `json:"code,omitempty"`
}

// Grant is one component's rights.
type Grant struct {
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Base        string   `json:"base,omitempty"`
	Lifecycle   string   `json:"lifecycle,omitempty"`
	Permissions string   `json:"permissions,omitempty"`
	Hosts       []string `json:"hosts,omitempty"`
	MCP         []string `json:"mcp,omitempty"`
	RequiresMCP []string `json:"requires_mcp,omitempty"`
	Sends       []string `json:"sends,omitempty"`
	Reads       bool     `json:"reads,omitempty"`
	Subscribes  []string `json:"subscribes,omitempty"`
}

// Code is an entry point Tracks runs as the user.
type Code struct {
	Path      string `json:"path"`
	Component string `json:"component"`
	Slot      string `json:"slot"` // "MCP server" or "context provider"
}

// ApprovalOf returns what installing m asks the user to approve, in an
// order of its own so the manifest's order doesn't count as a change.
func ApprovalOf(m Manifest) Approval {
	var a Approval
	for _, c := range m.Components {
		g := Grant{Kind: c.Kind, Name: c.Name, Base: c.Base, Lifecycle: c.Lifecycle, Permissions: c.Permissions,
			Hosts: sorted(c.Hosts), MCP: sorted(c.MCP), RequiresMCP: sorted(c.RequiresMCP)}
		if c.Relay != nil {
			g.Sends, g.Reads, g.Subscribes = sorted(c.Relay.Sends), c.Relay.Reads, sorted(c.Relay.Subscribes)
		}
		a.Components = append(a.Components, g)
		for _, p := range c.MCP {
			a.Code = append(a.Code, Code{Path: p, Component: c.Name, Slot: "MCP server"})
		}
		for _, p := range c.Context {
			a.Code = append(a.Code, Code{Path: p, Component: c.Name, Slot: "context provider"})
		}
	}
	slices.SortStableFunc(a.Components, func(x, y Grant) int {
		return cmp.Or(cmp.Compare(x.Kind, y.Kind), cmp.Compare(x.Name, y.Name))
	})
	slices.SortStableFunc(a.Code, func(x, y Code) int {
		return cmp.Or(cmp.Compare(x.Path, y.Path), cmp.Compare(x.Component, y.Component), cmp.Compare(x.Slot, y.Slot))
	})
	a.Code = slices.Compact(a.Code)
	return a
}

// Equal reports whether a and b grant the same.
func (a Approval) Equal(b Approval) bool {
	return slices.EqualFunc(a.Components, b.Components, Grant.equal) && slices.Equal(a.Code, b.Code)
}

func (g Grant) equal(o Grant) bool {
	return g.Kind == o.Kind && g.Name == o.Name && g.Base == o.Base && g.Lifecycle == o.Lifecycle &&
		g.Permissions == o.Permissions && g.Reads == o.Reads && slices.Equal(g.Hosts, o.Hosts) &&
		slices.Equal(g.MCP, o.MCP) && slices.Equal(g.RequiresMCP, o.RequiresMCP) &&
		slices.Equal(g.Sends, o.Sends) && slices.Equal(g.Subscribes, o.Subscribes)
}

// Widens lists what next grants that prev didn't, one line each.
func Widens(prev, next Approval) []string {
	var out []string
	for _, g := range next.Components {
		i := slices.IndexFunc(prev.Components, func(p Grant) bool { return p.Kind == g.Kind && p.Name == g.Name })
		if i < 0 {
			out = append(out, fmt.Sprintf("new %s %s", g.Kind, g.Name))
			continue
		}
		p := prev.Components[i]
		if rank(g.Permissions) > rank(p.Permissions) {
			out = append(out, fmt.Sprintf("%s: permissions %s, were %s", g.Name, g.Permissions, p.Permissions))
		}
		if g.Base != p.Base {
			out = append(out, fmt.Sprintf("%s: base %s, was %s", g.Name, g.Base, p.Base))
		}
		if g.Reads && !p.Reads {
			out = append(out, g.Name+": reads the relay")
		}
		for _, d := range []struct {
			what       string
			next, prev []string
		}{
			{"hosts", g.Hosts, p.Hosts},
			{"MCP servers shipped", g.MCP, p.MCP},
			{"MCP servers required", g.RequiresMCP, p.RequiresMCP},
			{"sends", g.Sends, p.Sends},
			{"subscribes to", g.Subscribes, p.Subscribes},
		} {
			if added := missing(d.next, d.prev); len(added) > 0 {
				out = append(out, fmt.Sprintf("%s: %s %s", g.Name, d.what, strings.Join(added, ", ")))
			}
		}
	}
	for _, c := range next.Code {
		if !slices.Contains(prev.Code, c) {
			out = append(out, "runs code: "+c.String())
		}
	}
	return out
}

func (c Code) String() string { return fmt.Sprintf("%s (%s of %s)", c.Path, c.Slot, c.Component) }

// rank orders permissions from the narrowest.
func rank(p string) int { return slices.Index(Permissions, p) }

// missing returns the values of vs that aren't in from.
func missing(vs, from []string) []string {
	var out []string
	for _, v := range vs {
		if !slices.Contains(from, v) {
			out = append(out, v)
		}
	}
	return out
}

func sorted(vs []string) []string {
	if len(vs) == 0 {
		return nil
	}
	out := slices.Clone(vs)
	slices.Sort(out)
	return slices.Compact(out)
}

// Describe is the approval summary shown before the user says yes.
func (a Approval) Describe() string {
	var b strings.Builder
	b.WriteString("Components:\n")
	for _, g := range a.Components {
		switch g.Kind {
		case Track:
			fmt.Fprintf(&b, "  track %s: a track type extending %s\n", g.Name, g.Base)
		default:
			perm := g.Permissions
			if perm == "write" {
				perm += " (runs as a side track)"
			}
			fmt.Fprintf(&b, "  train %s: %s, permissions %s\n", g.Name, g.Lifecycle, perm)
		}
		line(&b, "hosts", g.Hosts)
		line(&b, "MCP servers shipped", g.MCP)
		line(&b, "MCP servers required", g.RequiresMCP)
		if g.Kind == Train {
			fmt.Fprintf(&b, "    relay: %s\n", relayWords(g))
		}
	}
	if len(a.Code) == 0 {
		b.WriteString("Runs code: nothing\n")
		return b.String()
	}
	b.WriteString("Runs code, as you, once approved:\n")
	for _, c := range a.Code {
		fmt.Fprintf(&b, "  %s\n", c)
	}
	return b.String()
}

func line(b *strings.Builder, what string, vs []string) {
	if len(vs) > 0 {
		fmt.Fprintf(b, "    %s: %s\n", what, strings.Join(vs, ", "))
	}
}

func relayWords(g Grant) string {
	var parts []string
	if len(g.Sends) > 0 {
		parts = append(parts, "sends "+strings.Join(g.Sends, ", "))
	}
	if g.Reads {
		parts = append(parts, "reads what's addressed to it")
	}
	if len(g.Subscribes) > 0 {
		parts = append(parts, "subscribes to "+strings.Join(g.Subscribes, ", "))
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, "; ")
}

// Summary lists the components as "track custom-planner, train research".
func (a Approval) Summary() string {
	parts := make([]string, len(a.Components))
	for i, g := range a.Components {
		parts[i] = g.Kind + " " + g.Name
	}
	return strings.Join(parts, ", ")
}
