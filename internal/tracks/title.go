package tracks

import (
	"context"
	"strings"
)

// SetTitle gives track id the name title, trimmed; "" goes back to
// its slug. The slug, its window and branches keep their names.
func (s *Service) SetTitle(ctx context.Context, id, title string) error {
	return s.Store.SetTitle(ctx, id, strings.TrimSpace(title))
}
