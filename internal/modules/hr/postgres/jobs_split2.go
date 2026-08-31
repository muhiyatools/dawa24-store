package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// UpdateJobOffer modifies job vacancy details.
func (r *Repository) UpdateJobOffer(ctx context.Context, j *hr.JobOffer) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE hr.job_offers
			SET title = $1, description = $2, requirements = $3, salary_min = $4, salary_max = $5, location = $6, status = $7, updated_at = now()
			WHERE id = $8 AND organization_id = $9;
		`
		tag, err := tx.Exec(txCtx, query, j.Title, j.Description, j.Requirements, j.SalaryMin, j.SalaryMax, j.Location, j.Status, j.ID, j.OrganizationID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("job_offer")
		}
		return nil
	})
}

// DeleteJobOffer soft deletes a job vacancy.
func (r *Repository) DeleteJobOffer(ctx context.Context, orgID, jobID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `UPDATE hr.job_offers SET deleted_at = now(), updated_at = now() WHERE id = $1 AND organization_id = $2;`
		tag, err := tx.Exec(txCtx, query, jobID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("job_offer")
		}
		return nil
	})
}

// ToggleJobOfferStatus toggles status between 'published' and 'closed'.
func (r *Repository) ToggleJobOfferStatus(ctx context.Context, orgID, jobID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			UPDATE hr.job_offers
			SET status = CASE WHEN status = 'published' THEN 'closed' ELSE 'published' END,
			    updated_at = now()
			WHERE id = $1 AND organization_id = $2;
		`
		tag, err := tx.Exec(txCtx, query, jobID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("job_offer")
		}
		return nil
	})
}

// CountApplicationsByOffer returns total applicants for a job.
func (r *Repository) CountApplicationsByOffer(ctx context.Context, offerID int64) (int, error) {
	var count int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT COUNT(*) FROM hr.job_applications WHERE job_offer_id = $1;`
		return tx.QueryRow(txCtx, query, offerID).Scan(&count)
	})
	return count, err
}
