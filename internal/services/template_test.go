package services

import "testing"

func TestRenderPortAndFields(t *testing.T) {
	d := NewTemplateData("track-1", "/wt", map[string]int{"live-app": 20001})
	got, err := Render(`pnpm dev --port {{.Port "live-app"}} in {{.Worktree}}`, d)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "pnpm dev --port 20001 in /wt"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRenderUnknownPortErrors(t *testing.T) {
	d := NewTemplateData("t", "/wt", map[string]int{"a": 1})
	if _, err := Render(`{{.Port "missing"}}`, d); err == nil {
		t.Fatal("expected error for unknown port")
	}
}

func TestRenderParseError(t *testing.T) {
	d := NewTemplateData("t", "/wt", nil)
	if _, err := Render(`{{.Port`, d); err == nil {
		t.Fatal("expected parse error for malformed template")
	}
}

func TestRenderEnv(t *testing.T) {
	d := NewTemplateData("t", "/wt", map[string]int{"metro": 20007})
	got, err := RenderEnv(map[string]string{"RCT_METRO_PORT": `{{.Port "metro"}}`}, d)
	if err != nil {
		t.Fatalf("RenderEnv: %v", err)
	}
	if got["RCT_METRO_PORT"] != "20007" {
		t.Errorf("got %v", got)
	}
}

func TestRenderEnvEmpty(t *testing.T) {
	got, err := RenderEnv(nil, NewTemplateData("t", "/wt", nil))
	if err != nil {
		t.Fatalf("RenderEnv: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}
