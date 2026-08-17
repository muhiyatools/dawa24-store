package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

const serviceColumns = `id, public_id, title, description, icon, pricing_type, parent_id, is_active, created_at, updated_at`

// CreateService inserts an institutional service.
func (r *Repository) CreateService(ctx context.Context, s *workflow.InstitutionalService) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO workflow.services (title, description, icon, pricing_type, parent_id, is_active)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, s.Title, s.Description, s.Icon, string(s.PricingType), s.ParentID, s.IsActive).
			Scan(&s.ID, &s.PublicID, &s.CreatedAt, &s.UpdatedAt)
	})
}

// GetServiceByID fetches one institutional service.
func (r *Repository) GetServiceByID(ctx context.Context, id int64) (*workflow.InstitutionalService, error) {
	var s workflow.InstitutionalService
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT ` + serviceColumns + ` FROM workflow.services WHERE id = $1;`
		var pricing string
		err := tx.QueryRow(txCtx, query, id).Scan(&s.ID, &s.PublicID, &s.Title, &s.Description, &s.Icon, &pricing, &s.ParentID, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("service")
			}
			return err
		}
		s.PricingType = workflow.PricingType(pricing)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListServices returns services at a hierarchy level.
func (r *Repository) ListServices(ctx context.Context, parentID *int64) ([]*workflow.InstitutionalService, error) {
	var list []*workflow.InstitutionalService
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT ` + serviceColumns + ` FROM workflow.services WHERE is_active = true AND parent_id IS NOT DISTINCT FROM $1 ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query, parentID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var s workflow.InstitutionalService
			var pricing string
			if err := rows.Scan(&s.ID, &s.PublicID, &s.Title, &s.Description, &s.Icon, &pricing, &s.ParentID, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
				return err
			}
			s.PricingType = workflow.PricingType(pricing)
			list = append(list, &s)
		}
		return rows.Err()
	})
	return list, err
}
