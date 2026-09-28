package settings

import "slices"

// Engines are the agent CLIs set up on the Engines tab, by engine id.
// A nil one hasn't been added.
type Engines struct {
	Claude *Engine `yaml:"claude,omitempty"`
	Cursor *Engine `yaml:"cursor,omitempty"`
}

// Engine is one engine's settings.
type Engine struct {
	Model  string   `yaml:"model,omitempty"`  // the default model; "" is the engine's own
	Models []string `yaml:"models,omitempty"` // model ids the user added
	Auto   *bool    `yaml:"auto,omitempty"`   // nil means on
}

// AutoMode reports whether the engine runs without asking first.
func (e Engine) AutoMode() bool { return e.Auto == nil || *e.Auto }

// Get returns the engine id's settings, nil when it isn't added or
// Tracks doesn't know it.
func (e Engines) Get(id string) *Engine {
	switch id {
	case "claude":
		return e.Claude
	case "cursor":
		return e.Cursor
	}
	return nil
}

// Set replaces the engine id's settings; nil removes it.
func (e *Engines) Set(id string, engine *Engine) {
	switch id {
	case "claude":
		e.Claude = engine
	case "cursor":
		e.Cursor = engine
	}
}

// Clone copies e deeply, so the copy can be saved while e changes.
func (e Engines) Clone() Engines {
	return Engines{Claude: e.Claude.clone(), Cursor: e.Cursor.clone()}
}

func (e *Engine) clone() *Engine {
	if e == nil {
		return nil
	}
	c := *e
	c.Models = slices.Clone(e.Models)
	if e.Auto != nil {
		auto := *e.Auto
		c.Auto = &auto
	}
	return &c
}
