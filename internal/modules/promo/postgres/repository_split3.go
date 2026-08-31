package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

// ListAllSpecialOffers returns all special offers across suppliers with organizations, products and locations for admin.
func (r *Repository) ListAllSpecialOffers(ctx context.Context, limit, offset int) ([]*promo.SpecialOffer, error) {
	var list []*promo.SpecialOffer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT o.id, o.public_id, o.organization_id, COALESCE(org.legal_name, ''), o.branch_id, COALESCE(b.name->>'ar', ''),
			       o.title, o.description,
			       CASE WHEN o.discount_type = 'percentage' THEN o.discount_value ELSE 0 END,
			       CASE WHEN o.discount_type = 'fixed'      THEN o.discount_value ELSE 0 END,
			       COALESCE(o.min_order_amount, 0), COALESCE(o.total_price, 0),
			       o.starts_at, o.expires_at,
			       CASE WHEN o.is_draft   THEN 'draft'
			            WHEN o.is_active  THEN 'active'
			            ELSE 'inactive' END,
			       o.admin_status, COALESCE(o.image, ''),
			       o.created_at, o.updated_at
			FROM promo.offers o
			LEFT JOIN org.organizations org ON org.id = o.organization_id
			LEFT JOIN org.branches b ON b.id = o.branch_id
			WHERE o.deleted_at IS NULL
			ORDER BY o.created_at DESC
			LIMIT $1 OFFSET $2;
		`
		if limit <= 0 || limit > 200 {
			limit = 100
		}
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o promo.SpecialOffer
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.OrganizationID, &o.OrganizationName, &o.BranchID, &o.BranchName,
				&o.Title, &o.Description, &o.DiscountPercentage,
				&o.DiscountAmount, &o.MinOrderAmount, &o.TotalPrice,
				&o.StartDate, &o.EndDate, &o.Status, &o.AdminStatus, &o.Image,
				&o.CreatedAt, &o.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &o)
		}
		if err := rows.Err(); err != nil {
			return err
		}

		// Populate products and locations for each offer
		for _, offer := range list {
			pRows, _ := tx.Query(txCtx, `
				SELECT p.id, p.offer_id,
				       COALESCE(p.product_id, pv.product_id, 0),
				       COALESCE(p.variant_id, pv.id, 0),
				       COALESCE(prod.name->>'ar', prod.name->>'en', pv.sku, 'صنف دوائي معتمد'),
				       COALESCE(pv.price, prod.base_price, 0),
				       COALESCE(p.custom_price, 0),
				       COALESCE(p.custom_discount_percentage, 0),
				       COALESCE(p.custom_discount_amount, 0),
				       COALESCE(NULLIF(p.custom_qty, 0), 1),
				       p.created_at
				FROM promo.offer_products p
				LEFT JOIN catalog.product_variants pv ON (pv.id = p.variant_id OR (p.variant_id IS NULL AND pv.product_id = p.product_id))
				LEFT JOIN catalog.products prod ON prod.id = COALESCE(p.product_id, pv.product_id)
				WHERE p.offer_id = $1;
			`, offer.ID)
			if pRows != nil {
				for pRows.Next() {
					var p promo.SpecialOfferProduct
					if err := pRows.Scan(
						&p.ID, &p.OfferID, &p.ProductID, &p.VariantID, &p.VariantName, &p.OriginalPrice,
						&p.CustomPrice, &p.DiscountPercentage, &p.DiscountAmount, &p.Quantity, &p.CreatedAt,
					); err == nil {
						offer.Products = append(offer.Products, &p)
					}
				}
				pRows.Close()
			}

			lRows, _ := tx.Query(txCtx, `
				SELECT l.id, l.offer_id, l.city_id, COALESCE(c.name->>'ar', ''),
				       l.address_ar, l.address_en, l.latitude, l.longitude, l.radius_meters,
				       l.day_of_week + 1, COALESCE(to_char(l.time_from, 'HH24:MI'), ''), COALESCE(to_char(l.time_to, 'HH24:MI'), ''),
				       l.status, l.admin_status, l.created_at
				FROM promo.offer_location_covers l
				LEFT JOIN platform_admin.cities c ON c.id = l.city_id
				WHERE l.offer_id = $1;
			`, offer.ID)
			if lRows != nil {
				for lRows.Next() {
					var loc promo.SpecialOfferLocation
					if err := lRows.Scan(
						&loc.ID, &loc.OfferID, &loc.CityID, &loc.CityName,
						&loc.AddressAr, &loc.AddressEn, &loc.Latitude, &loc.Longitude, &loc.Radius,
						&loc.DayOfWeek, &loc.TimeFrom, &loc.TimeTo,
						&loc.Status, &loc.AdminStatus, &loc.CreatedAt,
					); err == nil {
						offer.Locations = append(offer.Locations, &loc)
					}
				}
				lRows.Close()
			}
		}

		return nil
	})
	return list, err
}

// UpdateSpecialOfferAdminStatus updates the moderation state (approved/rejected) of a special offer.
func (r *Repository) UpdateSpecialOfferAdminStatus(ctx context.Context, id int64, adminStatus, notes string, approvedBy int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE promo.offers
			SET admin_status = $1, admin_notes = $2, approved_by = $3,
			    approved_at = CASE WHEN $1 = 'approved' THEN now() ELSE approved_at END,
			    rejected_at = CASE WHEN $1 = 'rejected' THEN now() ELSE rejected_at END,
			    updated_at = now()
			WHERE id = $4;
		`
		tag, err := tx.Exec(txCtx, query, adminStatus, notes, approvedBy, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("special_offer")
		}
		return nil
	})
}

// ToggleSpecialOfferStatus activates or deactivates a special offer.
func (r *Repository) ToggleSpecialOfferStatus(ctx context.Context, id int64, isActive bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `UPDATE promo.offers SET is_active = $1, updated_at = now() WHERE id = $2;`, isActive, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("special_offer")
		}
		return nil
	})
}
