package postgres

import (
	"context"
	"fmt"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// CreateSponsorshipPurchase inserts a new sponsorship package purchase.
func (r *Repository) CreateSponsorshipPurchase(ctx context.Context, p *promo.SponsorshipPurchase) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.sponsorship_purchases (
				organization_id, package_id, credits_total, credits_used,
				starts_at, expires_at, status, auto_renew, billing_cycle,
				amount, payment_id, source_system, source_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			p.OrganizationID, p.PackageID, p.CreditsTotal, p.CreditsUsed,
			p.StartsAt, p.ExpiresAt, string(p.Status), p.AutoRenew, p.BillingCycle,
			p.Amount, p.PaymentID, p.SourceSystem, p.SourceID,
		).Scan(&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt)
	})
}

// GetSponsorshipPurchaseByID retrieves a purchase by ID.
func (r *Repository) GetSponsorshipPurchaseByID(ctx context.Context, id int64) (*promo.SponsorshipPurchase, error) {
	var p promo.SponsorshipPurchase
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, package_id, credits_total, credits_used,
			       starts_at, expires_at, status, auto_renew, billing_cycle, amount,
			       payment_id, source_system, source_id, approved_by, approved_at, created_at, updated_at
			FROM promo.sponsorship_purchases WHERE id = $1;
		`
		return tx.QueryRow(txCtx, query, id).Scan(
			&p.ID, &p.PublicID, &p.OrganizationID, &p.PackageID, &p.CreditsTotal, &p.CreditsUsed,
			&p.StartsAt, &p.ExpiresAt, &p.Status, &p.AutoRenew, &p.BillingCycle, &p.Amount,
			&p.PaymentID, &p.SourceSystem, &p.SourceID, &p.ApprovedBy, &p.ApprovedAt, &p.CreatedAt, &p.UpdatedAt,
		)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("sponsorship_purchase")
		}
		return nil, err
	}
	return &p, nil
}

// ListSponsorshipPurchasesByOrg returns all purchases for an organization.
func (r *Repository) ListSponsorshipPurchasesByOrg(ctx context.Context, orgID int64) ([]*promo.SponsorshipPurchase, error) {
	return r.listSponsorshipPurchases(ctx, orgID, false)
}

// ListActiveSponsorshipPurchasesByOrg returns only active purchases.
func (r *Repository) ListActiveSponsorshipPurchasesByOrg(ctx context.Context, orgID int64) ([]*promo.SponsorshipPurchase, error) {
	return r.listSponsorshipPurchases(ctx, orgID, true)
}

func (r *Repository) listSponsorshipPurchases(ctx context.Context, orgID int64, activeOnly bool) ([]*promo.SponsorshipPurchase, error) {
	var list []*promo.SponsorshipPurchase
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, package_id, credits_total, credits_used,
			       starts_at, expires_at, status, auto_renew, billing_cycle, amount,
			       payment_id, source_system, source_id, approved_by, approved_at, created_at, updated_at
			FROM promo.sponsorship_purchases
			WHERE organization_id = $1
		`
		if activeOnly {
			query += ` AND status = 'active' AND expires_at > now()`
		}
		query += ` ORDER BY created_at DESC;`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p promo.SponsorshipPurchase
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.OrganizationID, &p.PackageID, &p.CreditsTotal, &p.CreditsUsed,
				&p.StartsAt, &p.ExpiresAt, &p.Status, &p.AutoRenew, &p.BillingCycle, &p.Amount,
				&p.PaymentID, &p.SourceSystem, &p.SourceID, &p.ApprovedBy, &p.ApprovedAt, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &p)
		}
		return rows.Err()
	})
	return list, err
}

// IncrementSponsorshipPurchaseCreditsUsed atomically increments the credits
// used on a purchase, refusing if the result would exceed credits_total.
func (r *Repository) IncrementSponsorshipPurchaseCreditsUsed(ctx context.Context, purchaseID int64, credits int) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE promo.sponsorship_purchases
			SET credits_used = credits_used + $2, updated_at = now()
			WHERE id = $1
			  AND status = 'active'
			  AND credits_used + $2 <= credits_total
			  AND expires_at > now();
		`, purchaseID, credits)
		if err != nil {
			return fmt.Errorf("increment sponsorship credits: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.Conflict("sponsorship.insufficient_credits", i18n.TDefault("w4_mod.w4str_253_253"))
		}
		return nil
	})
}

// ExpireSponsorshipPurchases marks purchases past their expiry as expired.
func (r *Repository) ExpireSponsorshipPurchases(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE promo.sponsorship_purchases
			SET status = 'expired', updated_at = now()
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()
		return nil
	})
	return n, err
}

// --- Sponsorship requests ---

// CreateSponsorshipRequest inserts a new sponsorship request in pending status.
func (r *Repository) CreateSponsorshipRequest(ctx context.Context, sr *promo.SponsorshipRequest) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.sponsorship_requests (
				organization_id, purchase_id, package_id, item_type, item_id,
				credits_used, admin_status, starts_at, expires_at, status
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query,
			sr.OrganizationID, sr.PurchaseID, sr.PackageID, string(sr.ItemType), sr.ItemID,
			sr.CreditsUsed, string(sr.AdminStatus), sr.StartsAt, sr.ExpiresAt, string(sr.Status),
		).Scan(&sr.ID, &sr.PublicID, &sr.CreatedAt, &sr.UpdatedAt)
	})
}

// GetSponsorshipRequestByID retrieves a sponsorship request by ID.
func (r *Repository) GetSponsorshipRequestByID(ctx context.Context, id int64) (*promo.SponsorshipRequest, error) {
	var sr promo.SponsorshipRequest
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return scanSponsorshipRequest(tx.QueryRow(txCtx, sponsorshipRequestColumns+` WHERE id = $1;`, id), &sr)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("sponsorship_request")
		}
		return nil, err
	}
	return &sr, nil
}

// ListSponsorshipRequestsByOrg returns pagated requests for an organization.
func (r *Repository) ListSponsorshipRequestsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return r.listSponsorshipRequests(ctx, `WHERE organization_id = $1`, orgID, limit, offset)
}

// ListAllSponsorshipRequests returns all requests for admin moderation.
func (r *Repository) ListAllSponsorshipRequests(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return r.listSponsorshipRequests(database.AsSystem(ctx), ``, int64(0), limit, offset)
}

// ListPendingSponsorshipRequests returns only pending requests.
func (r *Repository) ListPendingSponsorshipRequests(ctx context.Context, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	return r.listSponsorshipRequests(database.AsSystem(ctx), `WHERE admin_status = 'pending'`, int64(0), limit, offset)
}

func (r *Repository) listSponsorshipRequests(ctx context.Context, where string, orgID int64, limit, offset int) ([]*promo.SponsorshipRequest, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list []*promo.SponsorshipRequest
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := sponsorshipRequestColumns + ` ` + where + ` ORDER BY created_at DESC LIMIT $` + whereParam(orgID) + ` OFFSET $` + whereParam2(orgID) + `;`
		var rows pgx.Rows
		var err error
		if orgID > 0 {
			rows, err = tx.Query(txCtx, query, orgID, limit, offset)
		} else {
			rows, err = tx.Query(txCtx, query, limit, offset)
		}
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sr promo.SponsorshipRequest
			if err := scanSponsorshipRequest(rows, &sr); err != nil {
				return err
			}
			list = append(list, &sr)
		}
		return rows.Err()
	})
	return list, err
}

func whereParam(orgID int64) string {
	if orgID > 0 {
		return "2"
	}
	return "1"
}
func whereParam2(orgID int64) string {
	if orgID > 0 {
		return "3"
	}
	return "2"
}

// UpdateSponsorshipRequestAdminStatus sets the admin status (approve/reject).
func (r *Repository) UpdateSponsorshipRequestAdminStatus(ctx context.Context, id int64, status promo.AdminStatus, notes string, reviewerID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE promo.sponsorship_requests
			SET admin_status = $1, admin_notes = $2, reviewed_by = $3, reviewed_at = now(), updated_at = now()
			WHERE id = $4;
		`, string(status), notes, reviewerID, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("sponsorship_request")
		}
		return nil
	})
}

// ActivateSponsorshipRequest approves a request, deducts credits from the
// purchase, and creates an offer_sponsorship row so the ranking query finds it.
func (r *Repository) ActivateSponsorshipRequest(ctx context.Context, id int64, reviewerID int64) (*promo.SponsorshipRequest, error) {
	var sr *promo.SponsorshipRequest
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(txCtx, sponsorshipRequestColumns+` WHERE id = $1 FOR UPDATE;`, id)
		sr = &promo.SponsorshipRequest{}
		if err := scanSponsorshipRequest(row, sr); err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("sponsorship_request")
			}
			return err
		}
		if sr.AdminStatus != "pending" {
			return apperr.Conflict("sponsorship.already_reviewed", i18n.TDefault("w4_mod.w4str_250_250"))
		}

		// Deduct credits from the purchase if linked.
		if sr.PurchaseID != nil {
			tag, err := tx.Exec(txCtx, `
				UPDATE promo.sponsorship_purchases
				SET credits_used = credits_used + $2, updated_at = now()
				WHERE id = $1 AND credits_used + $2 <= credits_total AND status = 'active';
			`, *sr.PurchaseID, sr.CreditsUsed)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return apperr.Conflict("sponsorship.insufficient_credits", i18n.TDefault("w4_mod.w4str_254_254"))
			}
		}

		// Mark as approved + active.
		tag, err := tx.Exec(txCtx, `
			UPDATE promo.sponsorship_requests
			SET admin_status = 'approved', status = 'active', reviewed_by = $2, reviewed_at = now(), updated_at = now()
			WHERE id = $1;
		`, id, reviewerID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("sponsorship_request")
		}

		// Create the offer_sponsorship row for ranking.
		_, err = tx.Exec(txCtx, `
			INSERT INTO promo.offer_sponsorships (
				organization_id, offer_id, package_id, starts_at, expires_at,
				status, item_type, item_id, credits_used, admin_status,
				reviewed_by, reviewed_at, sponsorship_request_id
			) VALUES ($1, $2, $3, $4, $5, 'active', $6, $7, $8, 'approved', $9, now(), $10)
			ON CONFLICT DO NOTHING;
		`, sr.OrganizationID, sr.ItemID, sr.PackageID, sr.StartsAt, sr.ExpiresAt,
			string(sr.ItemType), sr.ItemID, sr.CreditsUsed, reviewerID, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return sr, nil
}

// CancelSponsorshipRequest cancels a pending request owned by the org.
func (r *Repository) CancelSponsorshipRequest(ctx context.Context, id, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE promo.sponsorship_requests
			SET status = 'cancelled', updated_at = now()
			WHERE id = $1 AND organization_id = $2 AND admin_status = 'pending';
		`, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("sponsorship_request")
		}
		return nil
	})
}

// ExpireSponsorshipRequests marks requests past their expiry as expired.
func (r *Repository) ExpireSponsorshipRequests(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `
			UPDATE promo.sponsorship_requests
			SET status = 'expired', updated_at = now()
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return err
		}
		n = tag.RowsAffected()

		_, err = tx.Exec(txCtx, `
			UPDATE promo.offer_sponsorships
			SET status = 'expired'
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return err
		}
		return nil
	})
	return n, err
}

const sponsorshipRequestColumns = `
SELECT sr.id, sr.public_id, sr.organization_id, sr.purchase_id, sr.package_id,
       sr.item_type, sr.item_id, sr.credits_used, sr.admin_status, sr.admin_notes,
       sr.reviewed_by, sr.reviewed_at, sr.starts_at, sr.expires_at, sr.status,
       sr.created_at, sr.updated_at,
       op.tier_level, op.name, op.credits, op.duration_days
FROM promo.sponsorship_requests sr
LEFT JOIN promo.offer_packages op ON op.id = sr.package_id`

func scanSponsorshipRequest(row pgx.Row, sr *promo.SponsorshipRequest) error {
	var (
		pkgTier    *int
		pkgName    *[]byte
		pkgCredits *int
		pkgDur     *int
	)
	err := row.Scan(
		&sr.ID, &sr.PublicID, &sr.OrganizationID, &sr.PurchaseID, &sr.PackageID,
		&sr.ItemType, &sr.ItemID, &sr.CreditsUsed, &sr.AdminStatus, &sr.AdminNotes,
		&sr.ReviewedBy, &sr.ReviewedAt, &sr.StartsAt, &sr.ExpiresAt, &sr.Status,
		&sr.CreatedAt, &sr.UpdatedAt,
		&pkgTier, &pkgName, &pkgCredits, &pkgDur,
	)
	if err != nil {
		return err
	}
	if pkgTier != nil {
		sr.Package = &promo.OfferPackage{
			ID: sr.PackageID, TierLevel: *pkgTier, Credits: derefInt(pkgCredits), DurationDays: derefInt(pkgDur),
		}
	}
	return nil
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
