package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

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
