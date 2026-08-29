package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreatePlan inserts a subscription tier and its features.
func (r *Repository) CreatePlan(ctx context.Context, p *billing.Plan) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if p.MaxLoginSessions <= 0 {
			p.MaxLoginSessions = 3
		}
		if p.MaxDevices <= 0 {
			p.MaxDevices = 3
		}
		if p.AIPlanID == "" {
			p.AIPlanID = "plan-basic"
		}
		if p.IsDefault {
			_, _ = tx.Exec(txCtx, `UPDATE billing.plans SET is_default = false;`)
		}
		const query = `
			INSERT INTO billing.plans (slug, name, description, price_month, price_year, duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
			RETURNING id, public_id, created_at, updated_at;
		`
		if err := tx.QueryRow(txCtx, query, p.Slug, p.Name, p.Description, p.PriceMonth, p.PriceYear, p.DurationDays, p.MaxUsers, p.MaxLoginSessions, p.MaxDevices, p.AIPlanID, p.IsDefault, p.IsActive).
			Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return err
		}
		for key, val := range p.Features {
			if _, err := tx.Exec(txCtx, `INSERT INTO billing.plan_features (plan_id, feature_key, value) VALUES ($1, $2, $3) ON CONFLICT (plan_id, feature_key) DO UPDATE SET value = EXCLUDED.value;`, p.ID, key, val); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdatePlan updates an existing subscription tier.
func (r *Repository) UpdatePlan(ctx context.Context, p *billing.Plan) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if p.MaxLoginSessions <= 0 {
			p.MaxLoginSessions = 3
		}
		if p.MaxDevices <= 0 {
			p.MaxDevices = 3
		}
		if p.AIPlanID == "" {
			p.AIPlanID = "plan-basic"
		}
		if p.IsDefault {
			_, _ = tx.Exec(txCtx, `UPDATE billing.plans SET is_default = false WHERE id <> $1;`, p.ID)
		}
		const query = `
			UPDATE billing.plans
			SET name = $1, description = $2, price_month = $3, price_year = $4, duration_days = $5,
			    max_users = $6, max_login_sessions = $7, max_devices = $8, ai_plan_id = $9, is_default = $10,
			    is_active = $11, updated_at = now()
			WHERE id = $12;
		`
		tag, err := tx.Exec(txCtx, query, p.Name, p.Description, p.PriceMonth, p.PriceYear, p.DurationDays, p.MaxUsers, p.MaxLoginSessions, p.MaxDevices, p.AIPlanID, p.IsDefault, p.IsActive, p.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("plan")
		}
		for key, val := range p.Features {
			if _, err := tx.Exec(txCtx, `INSERT INTO billing.plan_features (plan_id, feature_key, value) VALUES ($1, $2, $3) ON CONFLICT (plan_id, feature_key) DO UPDATE SET value = EXCLUDED.value;`, p.ID, key, val); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetDefaultPlan retrieves the default subscription plan.
func (r *Repository) GetDefaultPlan(ctx context.Context) (*billing.Plan, error) {
	var p billing.Plan
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, slug, name, description, price_month, price_year,
			       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
			FROM billing.plans
			WHERE is_default = true AND is_active = true
			ORDER BY id ASC
			LIMIT 1;
		`
		err := tx.QueryRow(txCtx, query).Scan(
			&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
			&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
			&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
			&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil && database.IsNotFound(err) {
			// Fallback to first active plan
			fallbackQuery := `
				SELECT id, public_id, slug, name, description, price_month, price_year,
				       duration_days, max_users, max_login_sessions, max_devices, ai_plan_id, is_default, is_active, created_at, updated_at
				FROM billing.plans
				WHERE is_active = true
				ORDER BY id ASC
				LIMIT 1;
			`
			if err := tx.QueryRow(txCtx, fallbackQuery).Scan(
				&p.ID, &p.PublicID, &p.Slug, &p.Name, &p.Description,
				&p.PriceMonth, &p.PriceYear, &p.DurationDays, &p.MaxUsers,
				&p.MaxLoginSessions, &p.MaxDevices, &p.AIPlanID, &p.IsDefault,
				&p.IsActive, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		p.Features = loadPlanFeatures(txCtx, tx, p.ID)
		return nil
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("plan")
		}
		return nil, err
	}
	return &p, nil
}

// loadPlanFeatures reads the feature key-value pairs for a plan from billing.plan_features.
func loadPlanFeatures(ctx context.Context, tx pgx.Tx, planID int64) map[string]string {
	features := make(map[string]string)
	rows, err := tx.Query(ctx, `SELECT feature_key, value FROM billing.plan_features WHERE plan_id = $1;`, planID)
	if err != nil {
		return features
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			features[k] = v
		}
	}
	return features
}
