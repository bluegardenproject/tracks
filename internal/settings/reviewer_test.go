package settings

import "testing"

func TestReviewerOn(t *testing.T) {
	engines := Engines{Claude: &Engine{Model: "opus"}, Cursor: &Engine{Model: "gpt-5"}}
	for _, tc := range []struct {
		name                     string
		reviewer                 *Agent
		track, engine, wantModel string
	}{
		{"unset follows the track", nil, "cursor", "cursor", "gpt-5"},
		{"an empty engine follows the track", &Agent{}, "claude", "claude", "opus"},
		{"another engine, its default model", &Agent{Engine: "cursor"}, "claude", "cursor", "gpt-5"},
		{"another engine and model", &Agent{Engine: "cursor", Model: "gpt-5.6"}, "claude", "cursor", "gpt-5.6"},
		{"no track engine, the default", nil, "", "claude", "opus"},
		{"a prefix alone follows the track", &Agent{Prefix: "rev"}, "cursor", "cursor", "gpt-5"},
		{"a model without an engine is ignored", &Agent{Model: "sonnet"}, "claude", "claude", "opus"},
	} {
		s := Settings{Engines: engines, Agents: Agents{Reviewer: tc.reviewer}}
		if engine, model := s.ReviewerOn(tc.track); engine != tc.engine || model != tc.wantModel {
			t.Errorf("%s: ReviewerOn(%q) = %s, %s; want %s, %s", tc.name, tc.track, engine, model, tc.engine, tc.wantModel)
		}
	}
}
