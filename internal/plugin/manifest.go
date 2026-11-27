// Package plugin is what a plugin is: its manifest (plugin.yaml), the
// rules a manifest must follow, and what the user approves when
// installing one.
package plugin

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// File is the manifest's name in a plugin's folder.
const File = "plugin.yaml"

// The protocol version this Tracks speaks.
const (
	ProtocolMajor = 1
	ProtocolMinor = 0
)

// Engines a plugin may name; a manifest without engines is for all of
// them.
var Engines = []string{"claude", "cursor"}

// Component kinds.
const (
	Train = "train"
	Track = "track"
)

// Manifest is a plugin's plugin.yaml.
type Manifest struct {
	Protocol   Version     `yaml:"protocol"`
	Name       string      `yaml:"name"`
	Engines    []string    `yaml:"engines,omitempty"`
	Settings   []Setting   `yaml:"settings,omitempty"`
	Components []Component `yaml:"components"`
}

// Version is the protocol a manifest is written for, major.minor; a
// bare major means minor 0.
type Version struct {
	Major, Minor int
	Set          bool
}

func (v Version) String() string { return fmt.Sprintf("%d.%d", v.Major, v.Minor) }

// UnmarshalYAML reads 1, 1.2 or "1.2".
func (v *Version) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return &yaml.TypeError{Errors: []string{fmt.Sprintf("line %d: protocol is a version such as 1", n.Line)}}
	}
	major, minor, hasMinor := strings.Cut(strings.TrimSpace(n.Value), ".")
	var err error
	if v.Major, err = strconv.Atoi(major); err == nil && hasMinor {
		v.Minor, err = strconv.Atoi(minor)
	}
	if err != nil || v.Major < 0 || v.Minor < 0 {
		return &yaml.TypeError{Errors: []string{fmt.Sprintf("line %d: protocol %q isn't a version such as 1", n.Line, n.Value)}}
	}
	v.Set = true
	return nil
}

// Setting is one of the plugin's settings, drawn in the Plugins tab.
type Setting struct {
	Name string `yaml:"name"`
	// Type is string, bool or int.
	Type string `yaml:"type"`
	// Values, for a string, are the only ones it may take.
	Values  []string `yaml:"values,omitempty"`
	Default any      `yaml:"default,omitempty"`
}

// Component is a train or a track type the plugin brings. Some fields
// are for one kind only.
type Component struct {
	Kind    string   `yaml:"kind"`
	Name    string   `yaml:"name"`
	Brief   string   `yaml:"brief"`
	Context []string `yaml:"context,omitempty"` // context providers, code

	// A track type's.
	Base     string    `yaml:"base,omitempty"`
	Trains   []string  `yaml:"trains,omitempty"`
	Actions  []string  `yaml:"actions,omitempty"`
	Defaults *Defaults `yaml:"defaults,omitempty"`

	// A train's.
	Lifecycle   string   `yaml:"lifecycle,omitempty"`
	Permissions string   `yaml:"permissions,omitempty"`
	Hosts       []string `yaml:"hosts,omitempty"`
	MCP         []string `yaml:"mcp,omitempty"` // MCP servers it ships, code
	RequiresMCP []string `yaml:"requires_mcp,omitempty"`
	Relay       *Relay   `yaml:"relay,omitempty"`
}

// Defaults are a track type's engine and model.
type Defaults struct {
	Engine string `yaml:"engine,omitempty"`
	Model  string `yaml:"model,omitempty"`
}

// Relay is what a train does on its track's relay.
type Relay struct {
	Sends      []string `yaml:"sends,omitempty"`
	Reads      bool     `yaml:"reads,omitempty"`
	Subscribes []string `yaml:"subscribes,omitempty"`
}

// Problem is what's wrong with one field of a manifest.
type Problem struct {
	Field   string // such as components[1].permissions; "" for the whole file
	Message string
}

func (p Problem) String() string {
	if p.Field == "" {
		return p.Message
	}
	return p.Field + ": " + p.Message
}

// Problems are every problem found in a manifest.
type Problems []Problem

func (ps Problems) Error() string {
	lines := make([]string, len(ps))
	for i, p := range ps {
		lines[i] = p.String()
	}
	if len(lines) == 1 {
		return "plugin.yaml: " + lines[0]
	}
	return "plugin.yaml has " + strconv.Itoa(len(lines)) + " problems:\n  " + strings.Join(lines, "\n  ")
}

// Parse reads and checks a manifest. Warnings are worth showing but
// don't stop the plugin; a manifest with problems returns Problems.
func Parse(data []byte) (Manifest, []string, error) {
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, nil, decodeProblems(err)
	}
	warnings, problems := Check(m)
	if len(problems) > 0 {
		return Manifest{}, warnings, problems
	}
	return m, warnings, nil
}

var (
	unknownField = regexp.MustCompile(`field (\S+) not found in type \S+`)
	wrongType    = regexp.MustCompile("cannot unmarshal !!\\w+ (`[^`]*` )?into (\\S+)")
)

// typeWords says what a Go type is in a manifest's words.
func typeWords(goType string) string {
	switch {
	case goType == "bool":
		return "true or false"
	case goType == "int":
		return "a number"
	case goType == "string":
		return "text"
	case strings.HasPrefix(goType, "[]"):
		return "a list"
	}
	return "a mapping of fields"
}

// decodeProblems words yaml's errors for a plugin's author.
func decodeProblems(err error) error {
	var te *yaml.TypeError
	if !errors.As(err, &te) {
		if err.Error() == "EOF" {
			return Problems{{Message: "the file is empty"}}
		}
		return Problems{{Message: "isn't valid YAML: " + strings.TrimPrefix(err.Error(), "yaml: ")}}
	}
	var ps Problems
	for _, e := range te.Errors {
		e = unknownField.ReplaceAllString(e, "unknown field $1")
		e = wrongType.ReplaceAllStringFunc(e, func(s string) string {
			sub := wrongType.FindStringSubmatch(s)
			return sub[1] + "should be " + typeWords(sub[2])
		})
		ps = append(ps, Problem{Message: e})
	}
	return ps
}
