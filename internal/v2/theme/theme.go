// Package theme holds the design tokens and the colour values themes
// assign to them. It has no UI library imports: adapters elsewhere turn
// values into colours for Lip Gloss or tmux.
package theme

import (
	"embed"
	"fmt"
	"regexp"
	"slices"
	"sync"

	"gopkg.in/yaml.v3"
)

// Theme assigns a colour, "#rrggbb", to every token.
type Theme struct {
	// ID names the theme: its file name without ".yaml", or a
	// built-in's id.
	ID string
	// DisplayName is what Tracks shows for the theme: display_name in
	// its file, or the id when that's empty.
	DisplayName string
	BuiltIn     bool
	values      map[Token]string
}

// Value returns the colour for token.
func (t Theme) Value(token Token) string { return t.values[token] }

// DefaultID is the built-in theme used when none is chosen or the
// chosen one can't be read.
const DefaultID = "default"

// builtinIDs are the built-in themes in the order they're listed.
var builtinIDs = []string{DefaultID, "default_light"}

//go:embed themes/*.yaml
var builtinFiles embed.FS

var builtins = sync.OnceValue(func() []Theme {
	themes := make([]Theme, len(builtinIDs))
	for i, id := range builtinIDs {
		data, err := builtinFiles.ReadFile("themes/" + id + ".yaml")
		if err != nil {
			panic("built-in theme " + id + ": " + err.Error())
		}
		t, err := Parse(data)
		if err != nil {
			panic("built-in theme " + id + ": " + err.Error())
		}
		t.ID, t.BuiltIn = id, true
		themes[i] = t
	}
	return themes
})

// BuiltIns returns the built-in themes.
func BuiltIns() []Theme { return slices.Clone(builtins()) }

// Default is the Default theme.
func Default() Theme { return builtins()[0] }

func builtin(id string) (Theme, bool) {
	for _, t := range builtins() {
		if t.ID == id {
			return t, true
		}
	}
	return Theme{}, false
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Parse reads a theme file. It fails unless every token has a valid
// value and no unknown token appears.
func Parse(data []byte) (Theme, error) { return parse(data, nil) }

// parse reads a theme file; tokens it lacks come from base, if given.
func parse(data []byte, base *Theme) (Theme, error) {
	var file struct {
		Name   string            `yaml:"display_name"`
		Tokens map[string]string `yaml:"tokens"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return Theme{}, fmt.Errorf("not a theme file: %w", err)
	}
	t := Theme{DisplayName: file.Name, values: make(map[Token]string, len(All))}
	for name, v := range file.Tokens {
		token := Token(name)
		if !slices.Contains(All, token) {
			return Theme{}, fmt.Errorf("unknown token %q", name)
		}
		if !hexColor.MatchString(v) {
			return Theme{}, fmt.Errorf("token %q: %q is not #rrggbb", name, v)
		}
		t.values[token] = v
	}
	for _, token := range All {
		if _, ok := t.values[token]; ok {
			continue
		}
		if base == nil {
			return Theme{}, fmt.Errorf("token %q has no value", token)
		}
		t.values[token] = base.values[token]
	}
	return t, nil
}
