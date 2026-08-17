package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// CreatePlan inserts a subscription tier and its features.
func (r *Repository) CreatePlan(ctx context.Context, p *billing.Plan) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO billing.plans (slug, name, description, price_month, price_year, duration_days, max_users, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING id, public_id, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query, p.Slug, p.Name, p.Description, p.PriceMonth, p.PriceYear, p.DurationDays, p.MaxUsers, p.IsActive).
			Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return err
		}
		for key, val := range p.Features {
			if _, err := tx.Exec(txCtx, `INSERT INTO billing.plan_features (plan_id, feature_key, value) VALUES ($1, $2, $3) ON CONFLICT (plan_id, feature_key) DO NOTHING;`, p.ID, key, val); err != nil {
				return err
			}
		}
		return nil
	})
}
