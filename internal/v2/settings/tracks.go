package settings

// Tracks are each track type's defaults, by type. A nil one hasn't been
// set, and follows the Engines tab.
type Tracks struct {
	Work   *TrackType `yaml:"work,omitempty"`
	Ask    *TrackType `yaml:"ask,omitempty"`
	Plan   *TrackType `yaml:"plan,omitempty"`
	Review *TrackType `yaml:"review,omitempty"`
	Doc    *TrackType `yaml:"doc,omitempty"`
}

// TrackType is one track type's defaults: the engine its tracks run on,
// which may not be added, and the model, "" for the engine's default
// on the Engines tab.
type TrackType struct {
	Engine string `yaml:"engine"`
	Model  string `yaml:"model,omitempty"`
}

func (t *Tracks) field(kind string) **TrackType {
	switch kind {
	case "work":
		return &t.Work
	case "ask":
		return &t.Ask
	case "plan":
		return &t.Plan
	case "review":
		return &t.Review
	case "doc":
		return &t.Doc
	}
	return nil
}

// Get returns kind's defaults, nil when they aren't set.
func (t Tracks) Get(kind string) *TrackType {
	if f := t.field(kind); f != nil {
		return *f
	}
	return nil
}

// Set replaces kind's defaults; nil makes it follow the Engines tab.
func (t *Tracks) Set(kind string, d *TrackType) {
	if f := t.field(kind); f != nil {
		*f = d
	}
}

// RunsOn is what kind's tracks run on: its engine, else Claude when it's
// added, else Cursor; and its model, else that engine's default. The
// engine is "" with none added, and one kind names may not be added.
// The model is "" for the engine's own.
func (s Settings) RunsOn(kind string) (engine, model string) {
	d := s.Tracks.Get(kind)
	engine = s.Engines.DefaultID()
	if d != nil {
		engine = d.Engine
	}
	if d != nil && d.Model != "" {
		return engine, d.Model
	}
	if e := s.Engines.Get(engine); e != nil {
		return engine, e.Model
	}
	return engine, ""
}
