package settings

import (
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// merge writes the mapping ours, the encoding of a value of struct type
// t, into the mapping dst. Keys t declares take ours' value, or are
// removed when ours omits them; other keys stay as they are. Nested
// structs are merged the same way.
func merge(dst, ours *yaml.Node, t reflect.Type) {
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}
		ft := t.Field(i).Type
		value, ok := lookup(ours, name)
		// An omitted struct is its zero value, whose unknown keys stay; an
		// omitted pointer is nothing, and goes with everything under it.
		zero := !ok && ft.Kind() == reflect.Struct
		if zero {
			value, ok = &yaml.Node{Kind: yaml.MappingNode}, true
		}
		if !ok {
			remove(dst, name)
			continue
		}
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		old, had := lookup(dst, name)
		switch {
		case had && ft.Kind() == reflect.Struct && old.Kind == yaml.MappingNode && value.Kind == yaml.MappingNode:
			// An empty mapping reads back as {}; once it has keys they
			// read better as a block.
			if len(old.Content) == 0 {
				old.Style &^= yaml.FlowStyle
			}
			merge(old, value, ft)
			if zero && len(old.Content) == 0 {
				remove(dst, name)
			}
		case !zero:
			set(dst, name, value)
		}
	}
}

func lookup(m *yaml.Node, key string) (*yaml.Node, bool) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1], true
		}
	}
	return nil, false
}

// set puts key: value into the mapping m, replacing an existing value
// and keeping its comments.
func set(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			value.HeadComment, value.LineComment = m.Content[i+1].HeadComment, m.Content[i+1].LineComment
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func remove(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}
