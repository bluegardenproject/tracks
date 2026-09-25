package theme

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ValidHex reports whether s is a colour value themes accept.
func ValidHex(s string) bool { return hexColor.MatchString(s) }

// With returns a copy of t with token set to v.
func (t Theme) With(token Token, v Value) Theme {
	values := make(map[Token]Value, len(t.values))
	for k, old := range t.values {
		values[k] = old
	}
	values[token] = v
	return Theme{Name: t.Name, values: values}
}

// YAML writes t in the format of the built-in theme files, tokens in
// the order of All.
func (t Theme) YAML() []byte {
	width := 0
	for _, token := range All {
		width = max(width, len(token))
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "name: %s\ntokens:\n", t.Name)
	for _, token := range All {
		v := t.values[token]
		pad := strings.Repeat(" ", width-len(token))
		fmt.Fprintf(&b, "  %s: %s{ dark: %q, light: %q }\n", token, pad, v.Dark, v.Light)
	}
	return b.Bytes()
}

// Load reads the theme file at path. It returns the built-in theme when
// there is none, and along with the error when it can't be used.
func Load(path string) (Theme, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	t, err := Parse(data)
	if err != nil {
		return Default(), err
	}
	return t, nil
}

// Save writes t to path.
func (t Theme) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, t.YAML(), 0o644)
}
