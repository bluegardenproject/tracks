// Package demo fills the playground session (`tracks --new-app --demo`)
// with fake tracks. Only the content is fake: windows and panes are
// built by the real code, so the playground shows the real layout.
package demo

// Track is a fake track.
type Track struct {
	Name      string
	Repo      string
	Branch    string
	Kind      string // feature, fix, review
	Engine    string
	Model     string
	Session   string
	Cost      float64
	PR        int // 0 without a pull request
	PRState   string
	Scenario  Scenario
	DevServer *DevServer
}

// DevServer is a fake dev server shown in the track's right column.
type DevServer struct {
	Name string
	Port int
}

// Tracks lists the playground's tracks in window order.
func Tracks() []Track {
	return []Track{
		{Name: "api-auth", Repo: "shop-api", Branch: "feat/api-auth", Kind: "feature",
			Engine: "Claude", Model: "opus", Session: "3f9c2a1e-7b4d-4c8e-9a51-0d6e2f7b8c34", Cost: 2.84,
			Scenario: Implementing, DevServer: &DevServer{Name: "api", Port: 8080}},
		{Name: "checkout-flicker", Repo: "shop-web", Branch: "fix/checkout-flicker", Kind: "fix",
			Engine: "Cursor", Model: "gpt-5", Session: "b41d7e09-2c6a-4f13-8e7d-5a9c0b3f1e62", Cost: 0.97,
			PR: 1431, PRState: "draft", Scenario: Question, DevServer: &DevServer{Name: "web", Port: 3000}},
		{Name: "review-1423", Repo: "shop-web", Branch: "review/1423", Kind: "review",
			Engine: "Claude", Model: "sonnet", Session: "9e2b6c4d-0f18-4a7e-b3c5-7d1e8a2f6b90", Cost: 0.41,
			PR: 1423, PRState: "open", Scenario: Review},
		{Name: "docs-onboarding", Repo: "handbook", Branch: "docs/onboarding", Kind: "feature",
			Engine: "Claude", Model: "haiku", Session: "c7a0e5f2-8d3b-4e69-a1f4-2b6d9c0e7a15", Cost: 1.63,
			Scenario: Implementing},
	}
}
