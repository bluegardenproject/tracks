// Package settings reads and writes the user's preferences in
// settings.yaml. Saving keeps keys this build doesn't know, so older
// and newer builds can share the file.
package settings

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Settings are the preferences. The zero value is the defaults.
type Settings struct {
	Theme string `yaml:"theme,omitempty"` // the chosen theme's id
}

// Load reads the file at path. A missing file gives the defaults.
func Load(path string) (Settings, error) {
	var s Settings
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return Settings{}, fmt.Errorf("read %s: %w", path, err)
	}
	return s, nil
}

// Save writes s to path, keeping the keys and comments of the file
// that's there.
func Save(path string, s Settings) error {
	var doc yaml.Node
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return err
	default:
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
	}
	if doc.Kind == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("read %s: not a mapping", path)
	}
	var ours yaml.Node
	if err := ours.Encode(s); err != nil {
		return err
	}
	for i := 0; i+1 < len(ours.Content); i += 2 {
		set(root, ours.Content[i], ours.Content[i+1])
	}
	if s.Theme == "" {
		remove(root, "theme")
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return write(path, out)
}

// set puts key: value into the mapping m, replacing an existing value.
func set(m, key, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key.Value {
			value.HeadComment, value.LineComment = m.Content[i+1].HeadComment, m.Content[i+1].LineComment
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, key, value)
}

func remove(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

// write replaces path atomically.
func write(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
