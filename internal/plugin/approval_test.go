package plugin

import (
	"slices"
	"strings"
	"testing"
)

func TestApprovalOf(t *testing.T) {
	m := valid()
	m.Components[0].Context = []string{"ctx/adrs"}
	a := ApprovalOf(m)
	if a.Summary() != "track custom-planner, train research" {
		t.Errorf("summary %q", a.Summary())
	}
	want := []Code{{"ctx/adrs", "custom-planner", "context provider"}, {"servers/jira", "research", "MCP server"}}
	if !slices.Equal(a.Code, want) {
		t.Errorf("code %v", a.Code)
	}
	d := a.Describe()
	for _, s := range []string{
		"track custom-planner: a track type extending plan",
		"train research: one-shot, permissions read",
		"hosts: *.atlassian.net, api.example.com:8443",
		"MCP servers shipped: servers/jira",
		"MCP servers required: datadog",
		"relay: sends report; subscribes to github.ci_failed",
		"Runs code, as you, once approved:\n  ctx/adrs (context provider of custom-planner)\n  servers/jira (MCP server of research)",
	} {
		if !strings.Contains(d, s) {
			t.Errorf("description doesn't say %q:\n%s", s, d)
		}
	}
	m.Components[1].MCP, m.Components[0].Context = nil, nil
	if d := ApprovalOf(m).Describe(); !strings.Contains(d, "Runs code: nothing") {
		t.Errorf("description:\n%s", d)
	}
}

func TestWidens(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Manifest)
		want   []string
	}{
		{"same", func(*Manifest) {}, nil},
		{"narrower", func(m *Manifest) { m.Components[1].Hosts = nil }, nil},
		{"brief", func(m *Manifest) { m.Components[1].Brief = "other.md" }, nil},
		{"permissions", func(m *Manifest) { m.Components[1].Permissions = "write" }, []string{"research: permissions write, were read"}},
		{"fewer permissions", func(m *Manifest) { m.Components[1].Permissions = "read" }, nil},
		{"host", func(m *Manifest) { m.Components[1].Hosts = append(m.Components[1].Hosts, "x.io") }, []string{"research: hosts x.io"}},
		{"new train", func(m *Manifest) {
			m.Components = append(m.Components, Component{Kind: Train, Name: "watcher", Lifecycle: "long-running", Permissions: "read"})
		}, []string{"new train watcher"}},
		{"base", func(m *Manifest) { m.Components[0].Base = "work" }, []string{"custom-planner: base work, was plan"}},
		{"reads", func(m *Manifest) { m.Components[1].Relay.Reads = true }, []string{"research: reads the relay"}},
		{"sends", func(m *Manifest) { m.Components[1].Relay.Sends = []string{"question", "report"} }, []string{"research: sends question"}},
		{"code", func(m *Manifest) { m.Components[1].MCP = []string{"servers/jira", "servers/gh"} }, []string{
			"research: MCP servers shipped servers/gh", "runs code: servers/gh (MCP server of research)"}},
	}
	prev := ApprovalOf(valid())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid()
			tt.change(&m)
			next := ApprovalOf(m)
			if got := Widens(prev, next); !slices.Equal(got, tt.want) {
				t.Errorf("widens %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApprovalEqual(t *testing.T) {
	a := ApprovalOf(valid())
	m := valid()
	m.Components[1].Hosts = []string{"api.example.com:8443", "*.atlassian.net", "*.atlassian.net"}
	if !a.Equal(ApprovalOf(m)) {
		t.Error("the same hosts in another order differ")
	}
	m.Components[1].Hosts = nil
	if a.Equal(ApprovalOf(m)) {
		t.Error("fewer hosts are equal")
	}
}
