package plugin

import (
	"slices"
	"strings"
	"testing"
)

func valid() Manifest {
	return Manifest{
		Protocol: Version{Major: 1, Set: true},
		Name:     "acme-planning",
		Settings: []Setting{{Name: "adr_dir", Type: "string", Default: "docs/adr"}},
		Components: []Component{
			{Kind: Track, Name: "custom-planner", Base: "plan", Brief: "briefs/planner.md", Trains: []string{"research", "reviewer"}},
			{Kind: Train, Name: "research", Lifecycle: "one-shot", Permissions: "read", Brief: "briefs/research.md",
				Hosts: []string{"*.atlassian.net", "api.example.com:8443"}, MCP: []string{"servers/jira"},
				RequiresMCP: []string{"datadog"},
				Relay:       &Relay{Sends: []string{"report"}, Subscribes: []string{"github.ci_failed"}}},
		},
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Manifest)
		field  string // "" for a manifest without problems
		says   string
	}{
		{"valid", func(*Manifest) {}, "", ""},
		{"no protocol", func(m *Manifest) { m.Protocol = Version{} }, "protocol", "is missing"},
		{"other major", func(m *Manifest) { m.Protocol = Version{Major: 2, Set: true} }, "protocol", "speaks protocol 1 only"},
		{"no name", func(m *Manifest) { m.Name = "" }, "name", "is missing"},
		{"bad name", func(m *Manifest) { m.Name = "Acme Planning" }, "name", "lowercase letters"},
		{"long name", func(m *Manifest) { m.Name = strings.Repeat("a", 41) }, "name", "up to 40"},
		{"unknown engine", func(m *Manifest) { m.Engines = []string{"vim"} }, "engines[0]", "isn't an engine"},
		{"engine twice", func(m *Manifest) { m.Engines = []string{"claude", "claude"} }, "engines[1]", "twice"},
		{"setting name", func(m *Manifest) { m.Settings[0].Name = "ADR-dir" }, "settings[0].name", "lowercase"},
		{"setting twice", func(m *Manifest) { m.Settings = append(m.Settings, m.Settings[0]) }, "settings[1].name", "another setting"},
		{"setting type", func(m *Manifest) { m.Settings[0].Type = "float" }, "settings[0].type", "isn't a type"},
		{"default type", func(m *Manifest) { m.Settings[0].Default = 3 }, "settings[0].default", "doesn't fit"},
		{"default not a value", func(m *Manifest) { m.Settings[0].Values = []string{"a", "b"} }, "settings[0].default", "isn't one of a or b"},
		{"values on a bool", func(m *Manifest) {
			m.Settings[0] = Setting{Name: "x", Type: "bool", Values: []string{"a"}}
		}, "settings[0].values", "only string"},
		{"no components", func(m *Manifest) { m.Components = nil }, "components", "no components"},
		{"no kind", func(m *Manifest) { m.Components[0].Kind = "" }, "components[0].kind", "is missing"},
		{"bad kind", func(m *Manifest) { m.Components[0].Kind = "theme" }, "components[0].kind", "isn't a kind"},
		{"reserved name", func(m *Manifest) { m.Components[1].Name = "reviewer" }, "components[1].name", "Tracks' own"},
		{"relay handle name", func(m *Manifest) { m.Components[1].Name = "github" }, "components[1].name", "Tracks' own"},
		{"built-in kind name", func(m *Manifest) { m.Components[0].Name = "plan" }, "components[0].name", "Tracks' own"},
		{"same name", func(m *Manifest) { m.Components[0].Name = "research" }, "components[1].name", "another component"},
		{"no brief", func(m *Manifest) { m.Components[0].Brief = "" }, "components[0].brief", "is missing"},
		{"brief outside", func(m *Manifest) { m.Components[0].Brief = "../x.md" }, "components[0].brief", "inside the plugin's folder"},
		{"brief absolute", func(m *Manifest) { m.Components[0].Brief = "/etc/passwd" }, "components[0].brief", "inside"},
		{"context outside", func(m *Manifest) { m.Components[0].Context = []string{"a/../../b"} }, "components[0].context[0]", "inside"},
		{"context the folder", func(m *Manifest) { m.Components[0].Context = []string{"./"} }, "components[0].context[0]", "inside"},
		{"no base", func(m *Manifest) { m.Components[0].Base = "" }, "components[0].base", "is missing"},
		{"review base", func(m *Manifest) { m.Components[0].Base = "review" }, "components[0].base", "can't be extended yet"},
		{"unknown base", func(m *Manifest) { m.Components[0].Base = "deploy" }, "components[0].base", "isn't a track kind"},
		{"unknown train", func(m *Manifest) { m.Components[0].Trains = []string{"nope"} }, "components[0].trains[0]", "neither reviewer"},
		{"empty action", func(m *Manifest) { m.Components[0].Actions = []string{" "} }, "components[0].actions[0]", "is empty"},
		{"default engine", func(m *Manifest) {
			m.Engines = []string{"cursor"}
			m.Components[0].Defaults = &Defaults{Engine: "claude"}
		}, "components[0].defaults.engine", "isn't one of the plugin's engines (cursor)"},
		{"train field on a track", func(m *Manifest) { m.Components[0].Permissions = "read" }, "components[0].permissions", "train components only"},
		{"track field on a train", func(m *Manifest) { m.Components[1].Base = "plan" }, "components[1].base", "track components only"},
		{"no lifecycle", func(m *Manifest) { m.Components[1].Lifecycle = "" }, "components[1].lifecycle", "is missing"},
		{"bad lifecycle", func(m *Manifest) { m.Components[1].Lifecycle = "forever" }, "components[1].lifecycle", "isn't allowed"},
		{"no permissions", func(m *Manifest) { m.Components[1].Permissions = "" }, "components[1].permissions", "read, commands or write"},
		{"bad permissions", func(m *Manifest) { m.Components[1].Permissions = "root" }, "components[1].permissions", "isn't allowed"},
		{"host with scheme", func(m *Manifest) { m.Components[1].Hosts = []string{"https://x.com"} }, "components[1].hosts[0]", "isn't a host name"},
		{"host wildcard inside", func(m *Manifest) { m.Components[1].Hosts = []string{"a.*.com"} }, "components[1].hosts[0]", "isn't a host name"},
		{"mcp outside", func(m *Manifest) { m.Components[1].MCP = []string{"../srv"} }, "components[1].mcp[0]", "inside"},
		{"requires_mcp", func(m *Manifest) { m.Components[1].RequiresMCP = []string{"my server"} }, "components[1].requires_mcp[0]", "isn't an MCP server"},
		{"sends notice", func(m *Manifest) { m.Components[1].Relay.Sends = []string{"notice"} }, "components[1].relay.sends[0]", "Tracks' own"},
		{"sends unknown", func(m *Manifest) { m.Components[1].Relay.Sends = []string{"shout"} }, "components[1].relay.sends[0]", "isn't a kind of message"},
		{"subscribes", func(m *Manifest) { m.Components[1].Relay.Subscribes = []string{"CI failed"} }, "components[1].relay.subscribes[0]", "isn't an event"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := valid()
			tt.change(&m)
			_, problems := Check(m)
			if tt.field == "" {
				if len(problems) > 0 {
					t.Fatalf("problems: %v", problems)
				}
				return
			}
			i := slices.IndexFunc(problems, func(p Problem) bool { return p.Field == tt.field })
			if i < 0 {
				t.Fatalf("no problem with %s in %v", tt.field, problems)
			}
			if !strings.Contains(problems[i].Message, tt.says) {
				t.Errorf("%s: %q doesn't say %q", tt.field, problems[i].Message, tt.says)
			}
		})
	}
}

func TestCheckNewerMinorWarns(t *testing.T) {
	m := valid()
	m.Protocol = Version{Major: 1, Minor: 3, Set: true}
	warnings, problems := Check(m)
	if len(problems) > 0 || len(warnings) != 1 || !strings.Contains(warnings[0], "protocol 1.3") {
		t.Errorf("warnings %q, problems %v", warnings, problems)
	}
}

func TestInside(t *testing.T) {
	for p, want := range map[string]bool{
		"briefs/a.md": true, "a": true, "a/../b": true, "./a": true,
		"": false, ".": false, "..": false, "../a": false, "a/../../b": false, "/a": false, `a\b`: false,
	} {
		if got := Inside(p); got != want {
			t.Errorf("Inside(%q) = %v", p, got)
		}
	}
}
