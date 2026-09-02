package commerce

import (
	"context"
)

// AdminSearchOrders provides cross-tenant order search.
func (s *Service) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*Order, error) {
	return s.repo.AdminSearchOrders(ctx, query, limit, offset)
}

// AdminSearchOrdersWithTotal provides paginated cross-tenant order search with tab filter and total count.
func (s *Service) AdminSearchOrdersWithTotal(ctx context.Context, query, tab string, limit, offset int) ([]*Order, int, error) {
	return s.repo.AdminSearchOrdersWithTotal(ctx, query, tab, limit, offset)
}

// AdminOrderStats returns aggregated counts of all, direct, and negotiation orders.
func (s *Service) AdminOrderStats(ctx context.Context) (allCount, directCount, negotiationCount int, err error) {
	return s.repo.AdminOrderStats(ctx)
}
