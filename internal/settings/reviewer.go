package settings

// ReviewerOn is what `tracks review` runs on for a track on engine
// trackEngine: the Reviewer setting's engine, else the track's own,
// else the default engine; and the setting's model when it names an
// engine, else that engine's default. A model without an engine is
// ignored: models belong to an engine. The model is "" for the
// engine's own.
func (s Settings) ReviewerOn(trackEngine string) (engine, model string) {
	switch r := s.Agents.Reviewer; {
	case r != nil && r.Engine != "":
		if r.Model != "" {
			return r.Engine, r.Model
		}
		engine = r.Engine
	case trackEngine != "":
		engine = trackEngine
	default:
		engine = s.Engines.DefaultID()
	}
	if e := s.Engines.Get(engine); e != nil {
		return engine, e.Model
	}
	return engine, ""
}
