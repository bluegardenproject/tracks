// Package theme holds the design tokens and the colour values themes
// assign to them. It has no UI library imports: adapters elsewhere turn
// values into colours for Lip Gloss or tmux.
package theme

import (
	_ "embed"
	"fmt"
	"regexp"
	"slices"

	"gopkg.in/yaml.v3"
)

// Value is a token's colour on dark and on light terminal backgrounds,
// as "#rrggbb".
type Value struct {
	Dark  string `yaml:"dark"`
	Light string `yaml:"light"`
}

// For returns the variant for the terminal background.
func (v Value) For(dark bool) string {
	if dark {
		return v.Dark
	}
	return v.Light
}

// Theme assigns a value to every token.
type Theme struct {
	Name   string
	values map[Token]Value
}

// Value returns the value for token.
func (t Theme) Value(token Token) Value { return t.values[token] }

//go:embed themes/default.yaml
var defaultYAML []byte

// Default is the built-in theme.
func Default() Theme {
	t, err := Parse(defaultYAML)
	if err != nil {
		panic("built-in theme: " + err.Error())
	}
	return t
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Parse reads a theme file. It fails unless every token has a valid
// dark and light value and no unknown token appears.
func Parse(data []byte) (Theme, error) {
	var file struct {
		Name   string           `yaml:"name"`
		Tokens map[string]Value `yaml:"tokens"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return Theme{}, fmt.Errorf("parse theme: %w", err)
	}
	t := Theme{Name: file.Name, values: make(map[Token]Value, len(All))}
	for name, v := range file.Tokens {
		token := Token(name)
		if !slices.Contains(All, token) {
			return Theme{}, fmt.Errorf("theme %q: unknown token %q", file.Name, name)
		}
		for _, c := range []string{v.Dark, v.Light} {
			if !hexColor.MatchString(c) {
				return Theme{}, fmt.Errorf("theme %q: token %q: %q is not #rrggbb", file.Name, name, c)
			}
		}
		t.values[token] = v
	}
	for _, token := range All {
		if _, ok := t.values[token]; !ok {
			return Theme{}, fmt.Errorf("theme %q: token %q has no value", file.Name, token)
		}
	}
	return t, nil
}
