// Package services runs a track's declared dev servers: it renders their
// templated commands/env, starts them as supervised background processes,
// probes for readiness, and tears them down with the track.
//
// This file holds the templating layer only; process management and
// readiness probing land in later changes. Templates use Go's text/template
// with a small data context, e.g.:
//
//	cmd: 'pnpm dev --port {{.Port "live-app"}}'
//	env: { PORT: '{{.Port "live-app"}}' }
package services

import (
	"fmt"
	"strings"
	"text/template"
)

// TemplateData is the context available to service cmd/env/hook templates.
type TemplateData struct {
	// TrackID is the owning track's id, e.g. for {{.TrackID}}.
	TrackID string
	// Worktree is the absolute worktree path for this repo, {{.Worktree}}.
	Worktree string

	ports map[string]int
}

// NewTemplateData builds a render context for one track/worktree with the
// given allocated service ports.
func NewTemplateData(trackID, worktree string, ports map[string]int) TemplateData {
	return TemplateData{TrackID: trackID, Worktree: worktree, ports: ports}
}

// Port resolves the allocated port for a service by name, for use as
// {{.Port "name"}} in a template. Unknown names are a render error rather
// than a silent zero, so a typo fails loudly at start time.
func (d TemplateData) Port(name string) (int, error) {
	p, ok := d.ports[name]
	if !ok {
		return 0, fmt.Errorf("no port allocated for service %q", name)
	}
	return p, nil
}

// Render expands a single template string against the data. A reference to
// a missing field or an unknown port is an error.
func Render(tmpl string, data TemplateData) (string, error) {
	t, err := template.New("service").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse template %q: %w", tmpl, err)
	}
	var sb strings.Builder
	if err := t.Execute(&sb, data); err != nil {
		return "", fmt.Errorf("render template %q: %w", tmpl, err)
	}
	return sb.String(), nil
}

// RenderEnv renders every value in env, returning a new map. Keys are left
// untouched. A render error on any value aborts with that error.
func RenderEnv(env map[string]string, data TemplateData) (map[string]string, error) {
	if len(env) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(env))
	for k, v := range env {
		rendered, err := Render(v, data)
		if err != nil {
			return nil, fmt.Errorf("env %s: %w", k, err)
		}
		out[k] = rendered
	}
	return out, nil
}
