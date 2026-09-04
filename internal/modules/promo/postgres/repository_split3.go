package postgres

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ListAllSpecialOffers returns all special offers across suppliers with organizations, products and locations for admin.
func (r *Repository) ListAllSpecialOffers(ctx context.Context, limit, offset int) ([]*promo.SpecialOffer, error) {
	offers, _, err := r.ListAllSpecialOffersWithTotal(ctx, "", limit, offset)
	return offers, err
}

// ListAllSpecialOffersWithTotal returns a paginated slice of special offers with status filter and total count.
func (r *Repository) ListAllSpecialOffersWithTotal(ctx context.Context, statusFilter string, limit, offset int) ([]*promo.SpecialOffer, int, error) {
	var list []*promo.SpecialOffer
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		where := []string{"o.deleted_at IS NULL"}

		switch statusFilter {
		case "pending":
			where = append(where, "(o.admin_status = 'pending' OR o.admin_status IS NULL OR o.admin_status = '')")
		case "active":
			where = append(where, "(o.admin_status = 'approved' AND o.is_active = true AND o.is_draft = false)")
		case "rejected":
			where = append(where, "(o.admin_status = 'rejected')")
		case "draft":
			where = append(where, "(o.is_draft = true OR o.is_active = false)")
		}

		clause := ""
		for i, w := range where {
			if i > 0 {
				clause += " AND "
			}
			clause += w
		}

		countSQL := "SELECT count(*) FROM promo.offers o WHERE " + clause + ";"
		if err := tx.QueryRow(txCtx, countSQL).Scan(&total); err != nil {
			return err
		}

		if limit <= 0 || limit > 100 {
			limit = 25
		}
		if offset < 0 {
			offset = 0
		}

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
			       o.admin_status, COALESCE(o.admin_notes, ''), COALESCE(o.image, ''),
			       o.created_at, o.updated_at
			FROM promo.offers o
			LEFT JOIN org.organizations org ON org.id = o.organization_id
			LEFT JOIN org.branches b ON b.id = o.branch_id
			WHERE ` + clause + `
			ORDER BY o.created_at DESC, o.id DESC
			LIMIT $1 OFFSET $2;
		`
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
				&o.StartDate, &o.EndDate, &o.Status, &o.AdminStatus, &o.AdminNotes, &o.Image,
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
			prods, err := loadSpecialOfferProducts(txCtx, tx, offer.ID)
			if err != nil {
				return err
			}
			offer.Products = prods

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
	return list, total, err
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

// UpdateSpecialOffer updates a vendor's special offer and replaces bundled products, resetting admin_status to 'pending'.
func (r *Repository) UpdateSpecialOffer(ctx context.Context, o *promo.SpecialOffer) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		discType := "percentage"
		discVal := money.FromMinor(int64(math.Round(o.DiscountPercentage * 100)))
		if o.DiscountAmount.IsPositive() {
			discType = "fixed"
			discVal = o.DiscountAmount
		}

		query := `
			UPDATE promo.offers
			SET branch_id = $1, title = $2, description = $3,
			    discount_type = $4, discount_value = $5,
			    min_order_amount = $6, total_price = $7,
			    starts_at = COALESCE($8, starts_at),
			    expires_at = COALESCE($9, expires_at),
			    is_active = (COALESCE($10, 'active') = 'active'),
			    is_draft = (COALESCE($10, 'active') = 'draft'),
			    admin_status = 'pending',
			    image = CASE WHEN $11 <> '' THEN $11 ELSE image END,
			    updated_at = now()
			WHERE id = $12 AND organization_id = $13;
		`
		tag, err := tx.Exec(txCtx, query,
			o.BranchID, o.Title, o.Description,
			discType, discVal,
			o.MinOrderAmount, o.TotalPrice,
			o.StartDate, o.EndDate,
			o.Status, o.Image,
			o.ID, o.OrganizationID,
		)
		if err != nil {
			return fmt.Errorf("update special offer: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("special_offer")
		}

		// Replace products
		if _, err := tx.Exec(txCtx, `DELETE FROM promo.offer_products WHERE offer_id = $1;`, o.ID); err != nil {
			return fmt.Errorf("clear offer products: %w", err)
		}

		// A variant that is not this vendor's, or no longer exists, is skipped
		// rather than aborting the offer with a NOT NULL violation on product_id.
		if _, err := insertSpecialOfferProducts(txCtx, tx, o.ID, o.OrganizationID, o.Products); err != nil {
			return err
		}

		return nil
	})
}
