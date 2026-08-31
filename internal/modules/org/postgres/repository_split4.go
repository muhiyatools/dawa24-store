package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CountOrganizations returns how many organizations match the filter.
//
// The admin dashboard previously derived this from len() of a page capped at
// 100 rows, so every figure on it stopped counting at 100 and quietly
// under-reported from the hundred-and-first organization onward. A count
// belongs in SQL.
func (r *Repository) CountOrganizations(
	ctx context.Context,
	orgType *org.OrganizationType,
	status *org.OrganizationStatus,
) (int, error) {
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT COUNT(*)
			FROM org.organizations
			WHERE ($1::text IS NULL OR type = $1)
			  AND ($2::text IS NULL OR status = $2);
		`
		var typeStr, statusStr *string
		if orgType != nil {
			s := string(*orgType)
			typeStr = &s
		}
		if status != nil {
			s := string(*status)
			statusStr = &s
		}
		return tx.QueryRow(txCtx, query, typeStr, statusStr).Scan(&total)
	})
	return total, err
}

// GetDeliveryBands retrieves active delivery bands for an organization.
func (r *Repository) GetDeliveryBands(ctx context.Context, orgID int64) ([]*org.DeliveryBand, error) {
	var list []*org.DeliveryBand
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, organization_id, from_meters, to_meters, fee, is_active, created_at, updated_at FROM org.delivery_bands WHERE organization_id = $1 AND is_active = true ORDER BY from_meters ASC;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var b org.DeliveryBand
			if err := rows.Scan(&b.ID, &b.OrganizationID, &b.FromMeters, &b.ToMeters, &b.Fee, &b.IsActive, &b.CreatedAt, &b.UpdatedAt); err != nil {
				return err
			}
			list = append(list, &b)
		}
		return rows.Err()
	})
	return list, err
}

// SaveDeliveryBands replaces the delivery bands for an organization.
func (r *Repository) SaveDeliveryBands(ctx context.Context, orgID int64, bands []*org.DeliveryBand) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		delQuery := `DELETE FROM org.delivery_bands WHERE organization_id = $1;`
		if _, err := tx.Exec(txCtx, delQuery, orgID); err != nil {
			return err
		}

		insertQuery := `
			INSERT INTO org.delivery_bands (organization_id, from_meters, to_meters, fee, is_active)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, created_at, updated_at;
		`
		for _, b := range bands {
			b.OrganizationID = orgID
			err := tx.QueryRow(txCtx, insertQuery, orgID, b.FromMeters, b.ToMeters, b.Fee, b.IsActive).
				Scan(&b.ID, &b.CreatedAt, &b.UpdatedAt)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// GetReviewCriteria retrieves all evaluation criteria for a context (e.g. supplier, pharmacy, product).
func (r *Repository) GetReviewCriteria(ctx context.Context, contextType string) ([]*org.ReviewCriterion, error) {
	var list []*org.ReviewCriterion
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT key, name, context, weight, sort_order, is_active FROM org.review_criteria WHERE (context = $1 OR $1 = '') AND is_active = true ORDER BY sort_order ASC;`
		rows, err := tx.Query(txCtx, query, contextType)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var c org.ReviewCriterion
			if err := rows.Scan(&c.Key, &c.Name, &c.Context, &c.Weight, &c.SortOrder, &c.IsActive); err != nil {
				return err
			}
			list = append(list, &c)
		}
		return rows.Err()
	})
	return list, err
}

// AddReviewWithRatings adds a comprehensive multi-criteria review.
func (r *Repository) AddReviewWithRatings(ctx context.Context, rev *org.Review, ratings []org.ReviewRating) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO org.organization_reviews (
				organization_id, user_id, order_id, product_id, title, rating, review_text, is_verified, is_public, status, context
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			rev.OrganizationID, rev.UserID, rev.OrderID, rev.ProductID,
			rev.Title, rev.Rating, rev.ReviewText, rev.IsVerified, rev.IsPublic, rev.Status, rev.Context,
		).Scan(&rev.ID, &rev.PublicID, &rev.CreatedAt, &rev.UpdatedAt)
		if err != nil {
			return err
		}

		for _, rat := range ratings {
			queryRat := `INSERT INTO org.review_ratings (review_id, criterion, score) VALUES ($1, $2, $3);`
			if _, err := tx.Exec(txCtx, queryRat, rev.ID, rat.Criterion, rat.Score); err != nil {
				return err
			}
		}

		return nil
	})
}

// ReplyToReview adds a vendor reply to a review.
func (r *Repository) ReplyToReview(ctx context.Context, reviewID, orgID int64, response string, responderID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE org.organization_reviews
			SET response = $1, response_at = now(), responded_by = $2, updated_at = now()
			WHERE id = $3 AND organization_id = $4;
		`
		tag, err := tx.Exec(txCtx, query, response, responderID, reviewID, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("review")
		}
		return nil
	})
}
