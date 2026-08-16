package postgres

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
)

func (r *Repository) AdminSearchOrders(ctx context.Context, query string, limit, offset int) ([]*commerce.Order, error) {
	return nil, nil
}
