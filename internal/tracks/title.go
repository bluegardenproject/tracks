package tracks

import (
	"context"
	"strings"

	"github.com/bluegardenproject/tracks/internal/termtext"
)

// SetTitle gives track id the name title, trimmed and sanitized for
// single-line display; "" goes back to its slug. The slug, its window
// and branches keep their names.
func (s *Service) SetTitle(ctx context.Context, id, title string) error {
	return s.Store.SetTitle(ctx, id, strings.TrimSpace(termtext.SingleLine(title)))
}
