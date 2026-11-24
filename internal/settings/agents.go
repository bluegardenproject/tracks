package settings

// Agents are the agents Tracks starts beside a track's own, such as the
// reviewer: Settings → Trains (agents).
type Agents struct {
	// Reviewer is what `tracks review` runs on; nil, or no engine, for
	// the reviewed track's own engine.
	Reviewer *Agent `yaml:"reviewer,omitempty"`
}

// Agent is one of the agents beside a track's own: the engine and model
// it runs on, and the prefix of its trains' handles, "" for its name.
type Agent struct {
	Engine string `yaml:"engine,omitempty"`
	Model  string `yaml:"model,omitempty"`
	Prefix string `yaml:"prefix,omitempty"`
}

// ReviewerPrefix is the prefix the reviewer's handles take, "" for its
// name.
func (a Agents) ReviewerPrefix() string {
	if a.Reviewer == nil {
		return ""
	}
	return a.Reviewer.Prefix
}
