package settings

// ReviewerOn is what `tracks review` runs on for a track on engine
// trackEngine: the Reviewer setting's engine, else the track's own,
// else the default engine; and the setting's model, else that engine's
// default. The model is "" for the engine's own.
func (s Settings) ReviewerOn(trackEngine string) (engine, model string) {
	switch {
	case s.Reviewer != nil && s.Reviewer.Engine != "":
		engine = s.Reviewer.Engine
	case trackEngine != "":
		engine = trackEngine
	default:
		engine = s.Engines.DefaultID()
	}
	if s.Reviewer != nil && s.Reviewer.Model != "" {
		return engine, s.Reviewer.Model
	}
	if e := s.Engines.Get(engine); e != nil {
		return engine, e.Model
	}
	return engine, ""
}
