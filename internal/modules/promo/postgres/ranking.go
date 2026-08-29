package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// RankedSponsorshipsForProducts returns the active, approved sponsorships for
// the given product IDs, ranked by package tier level (highest first). Ties at
// the same tier are returned in arbitrary order — the caller shuffles them.
func (r *Repository) RankedSponsorshipsForProducts(ctx context.Context, productIDs []int64) ([]*promo.RankedSponsorship, error) {
	return r.rankedSponsorships(ctx, promo.SponsorItemProduct, productIDs)
}

// RankedSponsorshipsForOffers returns the active, approved sponsorships for
// the given offer IDs.
func (r *Repository) RankedSponsorshipsForOffers(ctx context.Context, offerIDs []int64) ([]*promo.RankedSponsorship, error) {
	return r.rankedSponsorships(ctx, promo.SponsorItemOffer, offerIDs)
}

func (r *Repository) rankedSponsorships(ctx context.Context, itemType promo.SponsorshipItemType, itemIDs []int64) ([]*promo.RankedSponsorship, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}
	var list []*promo.RankedSponsorship
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := fmt.Sprintf(`
			SELECT os.item_id, os.organization_id, os.package_id, op.tier_level, os.expires_at
			FROM promo.offer_sponsorships os
			JOIN promo.offer_packages op ON op.id = os.package_id
			WHERE os.status = 'active'
			  AND os.admin_status = 'approved'
			  AND os.item_type = $1
			  AND os.item_id = ANY($2)
			  AND os.expires_at > now()
			ORDER BY op.tier_level DESC, os.created_at ASC;
		`)
		rows, err := tx.Query(txCtx, query, string(itemType), itemIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var rs promo.RankedSponsorship
			if err := rows.Scan(&rs.ItemID, &rs.OrganizationID, &rs.PackageID, &rs.TierLevel, &rs.ExpiresAt); err != nil {
				return err
			}
			rs.ItemType = itemType
			list = append(list, &rs)
		}
		return rows.Err()
	})
	return list, err
}

// IsSponsored returns the highest-tier active sponsorship for a single item, or nil.
func (r *Repository) IsSponsored(ctx context.Context, itemType promo.SponsorshipItemType, itemID int64) (*promo.RankedSponsorship, error) {
	list, err := r.rankedSponsorships(ctx, itemType, []int64{itemID})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil
	}
	return list[0], nil
}
