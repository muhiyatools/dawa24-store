package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/hr"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

const jobColumns = `id, public_id, organization_id, category_id, title, description, requirements, salary_min, salary_max, location, status, created_at, updated_at`

// ListPublishedJobs returns published vacancies for the public job board.
func (r *Repository) ListPublishedJobs(ctx context.Context, limit, offset int) ([]*hr.JobOffer, error) {
	var list []*hr.JobOffer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT ` + jobColumns + ` FROM hr.job_offers WHERE status = 'published' AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2;`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var j hr.JobOffer
			if err := rows.Scan(&j.ID, &j.PublicID, &j.OrganizationID, &j.CategoryID, &j.Title, &j.Description, &j.Requirements, &j.SalaryMin, &j.SalaryMax, &j.Location, &j.Status, &j.CreatedAt, &j.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &j)
		}
		return rows.Err()
	})
	return list, err
}

// GetJobOfferByID fetches one vacancy.
func (r *Repository) GetJobOfferByID(ctx context.Context, id int64) (*hr.JobOffer, error) {
	var j hr.JobOffer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT ` + jobColumns + ` FROM hr.job_offers WHERE id = $1 AND deleted_at IS NULL;`
		err := tx.QueryRow(txCtx, query, id).Scan(&j.ID, &j.PublicID, &j.OrganizationID, &j.CategoryID, &j.Title, &j.Description, &j.Requirements, &j.SalaryMin, &j.SalaryMax, &j.Location, &j.Status, &j.CreatedAt, &j.UpdatedAt)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("job_offer")
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &j, nil
}

// CreateJobOffer inserts a vacancy.
func (r *Repository) CreateJobOffer(ctx context.Context, j *hr.JobOffer) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO hr.job_offers (organization_id, category_id, title, description, requirements, salary_min, salary_max, location, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, j.OrganizationID, j.CategoryID, j.Title, j.Description, j.Requirements, j.SalaryMin, j.SalaryMax, j.Location, j.Status).
			Scan(&j.ID, &j.PublicID, &j.CreatedAt, &j.UpdatedAt)
	})
}

// ListJobsByOrg returns a tenant's own postings.
func (r *Repository) ListJobsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*hr.JobOffer, error) {
	var list []*hr.JobOffer
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT ` + jobColumns + ` FROM hr.job_offers WHERE organization_id = $1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT $2 OFFSET $3;`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var j hr.JobOffer
			if err := rows.Scan(&j.ID, &j.PublicID, &j.OrganizationID, &j.CategoryID, &j.Title, &j.Description, &j.Requirements, &j.SalaryMin, &j.SalaryMax, &j.Location, &j.Status, &j.CreatedAt, &j.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &j)
		}
		return rows.Err()
	})
	return list, err
}

// CreateJobApplication inserts an application.
func (r *Repository) CreateJobApplication(ctx context.Context, a *hr.JobApplication) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `
			INSERT INTO hr.job_applications (job_offer_id, organization_id, applicant_name, applicant_email, applicant_phone, cv_storage_key, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, a.JobOfferID, a.OrganizationID, a.ApplicantName, a.ApplicantEmail, a.ApplicantPhone, a.CVStorageKey, a.Status).
			Scan(&a.ID, &a.PublicID, &a.CreatedAt, &a.UpdatedAt)
	})
}

// ListApplicationsByOffer returns applications for a vacancy.
func (r *Repository) ListApplicationsByOffer(ctx context.Context, offerID int64, limit, offset int) ([]*hr.JobApplication, error) {
	var list []*hr.JobApplication
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		const query = `SELECT id, public_id, job_offer_id, organization_id, applicant_name, applicant_email, applicant_phone, cv_storage_key, status, notes, created_at, updated_at FROM hr.job_applications WHERE job_offer_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3;`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, offerID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a hr.JobApplication
			if err := rows.Scan(&a.ID, &a.PublicID, &a.JobOfferID, &a.OrganizationID, &a.ApplicantName, &a.ApplicantEmail, &a.ApplicantPhone, &a.CVStorageKey, &a.Status, &a.Notes, &a.CreatedAt, &a.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &a)
		}
		return rows.Err()
	})
	return list, err
}
