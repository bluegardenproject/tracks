package main

import (
	"slices"
	"testing"
)

func TestNewAppRequested(t *testing.T) {
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == newAppEnv {
				return v
			}
			return ""
		}
	}
	tests := []struct {
		name   string
		args   []string
		env    string
		want   bool
		wantRe []string
	}{
		{"plain v1", []string{"ls"}, "", false, []string{"ls"}},
		{"flag alone", []string{"--new-app"}, "", true, []string{}},
		{"flag anywhere", []string{"demo", "--new-app", "stop"}, "", true, []string{"demo", "stop"}},
		{"env set", []string{"ls"}, "1", true, []string{"ls"}},
		{"env other value", []string{"ls"}, "0", false, []string{"ls"}},
		{"flag after -- is an argument", []string{"new", "--", "--new-app"}, "", false, []string{"new", "--", "--new-app"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rest := newAppRequested(tt.args, env(tt.env))
			if got != tt.want || !slices.Equal(rest, tt.wantRe) {
				t.Errorf("newAppRequested(%q, env=%q) = %v, %q; want %v, %q", tt.args, tt.env, got, rest, tt.want, tt.wantRe)
			}
		})
	}
}
