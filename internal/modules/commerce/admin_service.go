package commerce

import (
	"context"
)

// AdminSearchOrders provides cross-tenant order search.
func (s *Service) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*Order, error) {
	return s.repo.AdminSearchOrders(ctx, query, limit, offset)
}
