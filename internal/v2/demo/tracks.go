// Package demo fills the playground session (`tracks --new-app --demo`)
// with fake tracks. Only the content is fake: windows and panes are
// built by the real code, so the playground shows the real layout.
package demo

// Track is a fake track. It has no status yet; the status model is
// designed before chunk 2.
type Track struct {
	Name      string
	Repo      string
	Kind      string // feature, fix, review
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
		{Name: "api-auth", Repo: "shop-api", Kind: "feature", Scenario: Implementing,
			DevServer: &DevServer{Name: "api", Port: 8080}},
		{Name: "checkout-flicker", Repo: "shop-web", Kind: "fix", Scenario: Question,
			DevServer: &DevServer{Name: "web", Port: 3000}},
		{Name: "review-1423", Repo: "shop-web", Kind: "review", Scenario: Review},
		{Name: "docs-onboarding", Repo: "handbook", Kind: "feature", Scenario: Implementing},
	}
}
