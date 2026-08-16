package postgres

import (
	"context"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

func (r *Repository) AdminListSubscriptions(ctx context.Context, limit, offset int) ([]*billing.Subscription, error) {
	return nil, nil
}

func (r *Repository) AdminAdjustWallet(ctx context.Context, walletID int64, amount money.Amount, reason string, actorID int64) error {
	return nil
}

func (r *Repository) AdminListPayments(ctx context.Context, limit, offset int) ([]*billing.Payment, error) {
	return nil, nil
}
