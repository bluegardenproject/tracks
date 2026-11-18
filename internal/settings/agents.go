package settings

// Agents are the agents Tracks starts beside a track's own, such as the
// reviewer: Settings → Agents.
type Agents struct {
	// Reviewer is what `tracks review` runs on; nil, or no engine, for
	// the reviewed track's own engine.
	Reviewer *TrackType `yaml:"reviewer,omitempty"`
}
