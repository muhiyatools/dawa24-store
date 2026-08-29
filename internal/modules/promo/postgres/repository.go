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

// Repository implements promo.Repository using PostgreSQL.
type Repository struct {
	db *database.DB
}

// NewRepository creates a new promo PostgreSQL repository.
func NewRepository(db *database.DB) *Repository {
	return &Repository{db: db}
}

// CreateOffer inserts a promotion campaign for the active organization.
func (r *Repository) CreateOffer(ctx context.Context, o *promo.Offer) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.offers (
				organization_id, title, description, discount_type, discount_value,
				branch_id, admin_status, min_order_amount,
				starts_at, expires_at, is_active
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, public_id, created_at, updated_at;
		`
		if o.AdminStatus == "" {
			o.AdminStatus = "pending"
		}
		err := tx.QueryRow(txCtx, query,
			o.OrganizationID, o.Title, o.Description, string(o.DiscountType),
			o.DiscountValue, o.BranchID, o.AdminStatus,
			o.MinOrderAmount, o.StartsAt, o.ExpiresAt, o.IsActive,
		).Scan(&o.ID, &o.PublicID, &o.CreatedAt, &o.UpdatedAt)
		if err != nil {
			return fmt.Errorf("promo postgres: create offer: %w", err)
		}

		for _, prodID := range o.ProductIDs {
			_, err := tx.Exec(txCtx, `INSERT INTO promo.offer_products (offer_id, product_id) VALUES ($1, $2);`, o.ID, prodID)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// GetOfferByID retrieves an offer and its mapped product IDs.
func (r *Repository) GetOfferByID(ctx context.Context, id int64) (*promo.Offer, error) {
	var offer *promo.Offer
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		var err error
		offer, err = scanOffer(tx.QueryRow(txCtx, `
			SELECT `+offerColumns+`
			FROM promo.offers
			WHERE id = $1 AND deleted_at IS NULL;
		`, id))
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("offer")
			}
			return err
		}

		rows, err := tx.Query(txCtx, `SELECT product_id FROM promo.offer_products WHERE offer_id = $1;`, id)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var pID int64
			if err := rows.Scan(&pID); err != nil {
				return err
			}
			offer.ProductIDs = append(offer.ProductIDs, pID)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return offer, nil
}

// ListActiveOffers returns all approved offers currently running across vendors.
func (r *Repository) ListActiveOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	var offers []*promo.Offer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + offerColumns + `
			FROM promo.offers
			WHERE is_active = true AND admin_status = 'approved'
			  AND (starts_at IS NULL OR starts_at <= now())
			  AND (expires_at IS NULL OR expires_at >= now())
			  AND deleted_at IS NULL
			ORDER BY id DESC
			LIMIT $1 OFFSET $2;
		`
		if limit <= 0 || limit > 100 {
			limit = 20
		}
		rows, err := tx.Query(txCtx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			o, err := scanOffer(rows)
			if err != nil {
				return err
			}
			offers = append(offers, o)
		}
		return rows.Err()
	})
	return offers, err
}

// IncrementOfferEngagement atomically records views or clicks for an offer.
func (r *Repository) IncrementOfferEngagement(ctx context.Context, offerID int64, isClick bool) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		field := "views_count"
		if isClick {
			field = "clicks_count"
		}
		query := fmt.Sprintf(`UPDATE promo.offers SET %s = %s + 1 WHERE id = $1;`, field, field)
		_, err := tx.Exec(txCtx, query, offerID)
		return err
	})
}

// CreatePackage inserts a sponsorship package.
func (r *Repository) CreatePackage(ctx context.Context, p *promo.OfferPackage) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.offer_packages (name, price, duration_days, max_offers, is_active)
			VALUES ($1, $2, $3, $4, $5)
			RETURNING id, public_id, created_at, updated_at;
		`
		return tx.QueryRow(txCtx, query, p.Name, p.Price, p.DurationDays, p.MaxOffers, p.IsActive).Scan(
			&p.ID, &p.PublicID, &p.CreatedAt, &p.UpdatedAt,
		)
	})
}

// ListPackages returns all active packages.
func (r *Repository) ListPackages(ctx context.Context) ([]*promo.OfferPackage, error) {
	var packages []*promo.OfferPackage
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, name, price, duration_days, max_offers, is_active, created_at, updated_at FROM promo.offer_packages WHERE is_active = true ORDER BY id ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var p promo.OfferPackage
			if err := rows.Scan(
				&p.ID, &p.PublicID, &p.Name, &p.Price, &p.DurationDays, &p.MaxOffers, &p.IsActive, &p.CreatedAt, &p.UpdatedAt,
			); err != nil {
				return err
			}
			packages = append(packages, &p)
		}
		return rows.Err()
	})
	return packages, err
}

// CreateSponsorship links an offer to a paid package.
func (r *Repository) CreateSponsorship(ctx context.Context, s *promo.OfferSponsorship) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.offer_sponsorships (organization_id, offer_id, package_id, starts_at, expires_at, status)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, public_id, created_at;
		`
		return tx.QueryRow(txCtx, query, s.OrganizationID, s.OfferID, s.PackageID, s.StartsAt, s.ExpiresAt, s.Status).Scan(
			&s.ID, &s.PublicID, &s.CreatedAt,
		)
	})
}

// ListActiveAds returns display ads by screen position.
func (r *Repository) ListActiveAds(ctx context.Context, position string) ([]*promo.Ad, error) {
	var ads []*promo.Ad
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, title, image_url, target_url,
			       position, is_active, starts_at, expires_at, impressions, clicks, created_at, updated_at
			FROM promo.ads
			WHERE is_active = true AND starts_at <= now() AND expires_at >= now()
			  AND ($1 = '' OR position = $1)
			ORDER BY id DESC;
		`
		rows, err := tx.Query(txCtx, query, position)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var a promo.Ad
			if err := rows.Scan(
				&a.ID, &a.PublicID, &a.OrganizationID, &a.Title, &a.ImageURL, &a.TargetURL,
				&a.Position, &a.IsActive, &a.StartsAt, &a.ExpiresAt, &a.Impressions, &a.Clicks, &a.CreatedAt, &a.UpdatedAt,
			); err != nil {
				return err
			}
			ads = append(ads, &a)
		}
		return rows.Err()
	})
	return ads, err
}

// RecordAdClick logs click analytics and increments counters.
func (r *Repository) RecordAdClick(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `UPDATE promo.ads SET clicks = clicks + 1 WHERE id = $1;`, adID); err != nil {
			return err
		}
		query := `INSERT INTO promo.ad_clicks (ad_id, user_id, ip_address, user_agent) VALUES ($1, $2, $3, $4);`
		_, err := tx.Exec(txCtx, query, adID, userID, ip, ua)
		return err
	})
}

// CreateHighlightSection creates a homepage curated section. platform rows
// are created via the promo API; organization rows carry owner_type +
// organization_id (066).
func (r *Repository) CreateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.highlight_sections (title, description, section_type, color, slug, display_order, is_active, show_in_header, owner_type, organization_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING id, public_id, created_at, updated_at;
		`
		if h.OwnerType == "" {
			h.OwnerType = "platform"
		}
		if h.SectionType == "" {
			h.SectionType = "about"
		}
		if h.Color == "" {
			h.Color = "#0284c7"
		}
		return tx.QueryRow(txCtx, query,
			h.Title, h.Description, h.SectionType, h.Color,
			h.Slug, h.DisplayOrder, h.IsActive, h.ShowInHeader,
			h.OwnerType, h.OrganizationID,
		).Scan(&h.ID, &h.PublicID, &h.CreatedAt, &h.UpdatedAt)
	})
}

// ListHighlightSections returns all active platform curated sections.
func (r *Repository) ListHighlightSections(ctx context.Context) ([]*promo.HighlightSection, error) {
	var list []*promo.HighlightSection
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, title, slug, display_order, is_active, owner_type, organization_id, created_at FROM promo.highlight_sections WHERE is_active = true AND owner_type = 'platform' ORDER BY display_order ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var h promo.HighlightSection
			if err := rows.Scan(&h.ID, &h.PublicID, &h.Title, &h.Slug, &h.DisplayOrder, &h.IsActive, &h.OwnerType, &h.OrganizationID, &h.CreatedAt); err != nil {
				return err
			}
			list = append(list, &h)
		}
		return rows.Err()
	})
	return list, err
}

// ExpirePromotions marks expired offers and sponsorships as inactive.
func (r *Repository) ExpirePromotions(ctx context.Context) (int64, error) {
	var totalExpired int64
	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		tagOffers, err := tx.Exec(txCtx, `UPDATE promo.offers SET is_active = false, updated_at = now() WHERE is_active = true AND expires_at < now();`)
		if err != nil {
			return err
		}
		tagSponsors, err := tx.Exec(txCtx, `UPDATE promo.offer_sponsorships SET status = 'expired' WHERE status = 'active' AND expires_at < now();`)
		if err != nil {
			return err
		}
		tagAds, err := tx.Exec(txCtx, `UPDATE promo.ads SET is_active = false, updated_at = now() WHERE is_active = true AND expires_at < now();`)
		if err != nil {
			return err
		}
		totalExpired = tagOffers.RowsAffected() + tagSponsors.RowsAffected() + tagAds.RowsAffected()
		return nil
	})
	return totalExpired, err
}

// CreateSpecialOffer inserts the vendor's special offer onto promo.offers
// (Rebuild V2 §2.2 — the special_offers family merged into offers/065). The
// legacy status vocabulary maps onto the offers flags: draft -> is_draft,
// active -> is_active, anything else -> inactive.
func (r *Repository) CreateSpecialOffer(ctx context.Context, o *promo.SpecialOffer) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		discType := "percentage"
		discVal := money.FromMinor(int64(math.Round(o.DiscountPercentage * 100)))
		if o.DiscountAmount.IsPositive() {
			discType = "fixed"
			discVal = o.DiscountAmount
		}

		query := `
			INSERT INTO promo.offers (
				organization_id, branch_id, title, description,
				discount_type, discount_value, min_order_amount, total_price,
				starts_at, expires_at, is_active, is_draft, admin_status, image, source
			) VALUES (
				$1, $2, $3, $4,
				$5, $6, $7, $8, 
				COALESCE($9, now()), 
				COALESCE($10, now() + interval '1 year'),
				COALESCE($11, 'active') = 'active',
				COALESCE($11, 'active') = 'draft',
				COALESCE(NULLIF($12, ''), 'approved'), COALESCE($13, ''), 'special'
			)
			RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			o.OrganizationID, o.BranchID, o.Title, o.Description,
			discType, discVal,
			o.MinOrderAmount, o.TotalPrice, o.StartDate, o.EndDate,
			o.Status, o.AdminStatus, o.Image,
		).Scan(&o.ID, &o.PublicID, &o.CreatedAt, &o.UpdatedAt)
		if err != nil {
			return fmt.Errorf("create special offer: %w", err)
		}

		for _, p := range o.Products {
			pQuery := `
				INSERT INTO promo.offer_products (
					offer_id, product_id, variant_id, custom_price,
					custom_discount_percentage, custom_discount_amount, custom_qty
				) VALUES ($1, (SELECT product_id FROM catalog.product_variants WHERE id = $2), $2, $3, $4, $5, $6);
			`
			_, err := tx.Exec(txCtx, pQuery, o.ID, p.VariantID, p.CustomPrice, p.DiscountPercentage, p.DiscountAmount, p.Quantity)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

// GetSpecialOfferByID retrieves a special offer with its products and locations
// from the merged offers family (065). The legacy special_offers shape is
// reproduced: discount fields split back into percentage/amount, dates into
// start/end, and the status derives from is_active/is_draft.
func (r *Repository) GetSpecialOfferByID(ctx context.Context, id int64) (*promo.SpecialOffer, error) {
	var o promo.SpecialOffer
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT o.id, o.public_id, o.organization_id, COALESCE(org.name->>'ar', org.legal_name, 'مورد معتمد'), o.branch_id, COALESCE(b.name->>'ar', ''),
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
			WHERE o.id = $1 AND o.deleted_at IS NULL;
		`
		err := tx.QueryRow(txCtx, query, id).Scan(
			&o.ID, &o.PublicID, &o.OrganizationID, &o.OrganizationName, &o.BranchID, &o.BranchName,
			&o.Title, &o.Description, &o.DiscountPercentage,
			&o.DiscountAmount, &o.MinOrderAmount, &o.TotalPrice,
			&o.StartDate, &o.EndDate, &o.Status, &o.AdminStatus, &o.Image,
			&o.CreatedAt, &o.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("special_offer")
			}
			return err
		}

		// Load Products
		pRows, _ := tx.Query(txCtx, `
			SELECT p.id, p.offer_id, p.variant_id, COALESCE(prod.name->>'ar', prod.name->>'en', pv.sku, ''), COALESCE(pv.price, 0),
			       p.custom_price, p.custom_discount_percentage, p.custom_discount_amount, p.custom_qty, p.created_at
			FROM promo.offer_products p
			LEFT JOIN catalog.product_variants pv ON pv.id = p.variant_id
			LEFT JOIN catalog.products prod ON prod.id = pv.product_id
			WHERE p.offer_id = $1;
		`, id)
		if pRows != nil {
			for pRows.Next() {
				var p promo.SpecialOfferProduct
				if err := pRows.Scan(
					&p.ID, &p.OfferID, &p.VariantID, &p.VariantName, &p.OriginalPrice,
					&p.CustomPrice, &p.DiscountPercentage, &p.DiscountAmount, &p.Quantity, &p.CreatedAt,
				); err == nil {
					o.Products = append(o.Products, &p)
				}
			}
			pRows.Close()
		}

		// Load Locations
		lRows, _ := tx.Query(txCtx, `
			SELECT l.id, l.offer_id, l.city_id, COALESCE(c.name->>'ar', ''),
			       l.address_ar, l.address_en, l.latitude, l.longitude, l.radius_meters,
			       l.day_of_week + 1, COALESCE(to_char(l.time_from, 'HH24:MI'), ''), COALESCE(to_char(l.time_to, 'HH24:MI'), ''),
			       l.status, l.admin_status, l.created_at
			FROM promo.offer_location_covers l
			LEFT JOIN platform_admin.cities c ON c.id = l.city_id
			WHERE l.offer_id = $1;
		`, id)
		if lRows != nil {
			for lRows.Next() {
				var loc promo.SpecialOfferLocation
				if err := lRows.Scan(
					&loc.ID, &loc.OfferID, &loc.CityID, &loc.CityName,
					&loc.AddressAr, &loc.AddressEn, &loc.Latitude, &loc.Longitude, &loc.Radius,
					&loc.DayOfWeek, &loc.TimeFrom, &loc.TimeTo,
					&loc.Status, &loc.AdminStatus, &loc.CreatedAt,
				); err == nil {
					o.Locations = append(o.Locations, &loc)
				}
			}
			lRows.Close()
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListSpecialOffersByOrg returns all special offers for an organization from
// the merged offers family (065 records carry source = 'special').
func (r *Repository) ListSpecialOffersByOrg(ctx context.Context, orgID int64) ([]*promo.SpecialOffer, error) {
	var list []*promo.SpecialOffer
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT o.id, o.public_id, o.organization_id, o.branch_id, COALESCE(b.name->>'ar', ''),
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
			LEFT JOIN org.branches b ON b.id = o.branch_id
			WHERE o.organization_id = $1
			  AND o.source = 'special'
			  AND o.deleted_at IS NULL
			ORDER BY o.created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, orgID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var o promo.SpecialOffer
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.OrganizationID, &o.BranchID, &o.BranchName,
				&o.Title, &o.Description, &o.DiscountPercentage,
				&o.DiscountAmount, &o.MinOrderAmount, &o.TotalPrice,
				&o.StartDate, &o.EndDate, &o.Status, &o.AdminStatus, &o.Image,
				&o.CreatedAt, &o.UpdatedAt,
			); err != nil {
				return err
			}
			list = append(list, &o)
		}
		return rows.Err()
	})
	return list, err
}

// DeleteSpecialOffer deletes a special offer (cascades to its products and
// location covers through the offers family).
func (r *Repository) DeleteSpecialOffer(ctx context.Context, id, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `DELETE FROM promo.offers WHERE id = $1 AND organization_id = $2;`, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("special_offer")
		}
		return nil
	})
}

// AddSpecialOfferLocation inserts a geographic coverage entry for an offer.
// day_of_week shifts from the legacy 1..7 to the offers family 0..6 (0 = Sunday).
func (r *Repository) AddSpecialOfferLocation(ctx context.Context, loc *promo.SpecialOfferLocation) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.offer_location_covers (
				organization_id, offer_id, city_id, address_ar, address_en, latitude, longitude,
				radius_meters, day_of_week, time_from, time_to, status, admin_status
			) VALUES (
				(SELECT organization_id FROM promo.offers WHERE id = $1),
				$1, $2, $3, $4, $5, $6,
				COALESCE($7, 500), COALESCE($8, 1) - 1, NULLIF($9, '')::time, NULLIF($10, '')::time,
				COALESCE(NULLIF($11, ''), 'active'), COALESCE(NULLIF($12, ''), 'approved')
			)
			RETURNING id, created_at;
		`
		return tx.QueryRow(txCtx, query,
			loc.OfferID, loc.CityID, loc.AddressAr, loc.AddressEn, loc.Latitude, loc.Longitude,
			loc.Radius, loc.DayOfWeek, loc.TimeFrom, loc.TimeTo, loc.Status, loc.AdminStatus,
		).Scan(&loc.ID, &loc.CreatedAt)
	})
}

// ListSpecialOfferLocations lists coverage locations for an offer,
// restoring the legacy 1..7 day_of_week numbering.
func (r *Repository) ListSpecialOfferLocations(ctx context.Context, offerID int64) ([]*promo.SpecialOfferLocation, error) {
	var list []*promo.SpecialOfferLocation
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT l.id, l.offer_id, l.city_id, COALESCE(c.name->>'ar', ''),
			       l.address_ar, l.address_en, l.latitude, l.longitude, l.radius_meters,
			       l.day_of_week + 1, COALESCE(to_char(l.time_from, 'HH24:MI'), ''), COALESCE(to_char(l.time_to, 'HH24:MI'), ''),
			       l.status, l.admin_status, l.created_at
			FROM promo.offer_location_covers l
			LEFT JOIN platform_admin.cities c ON c.id = l.city_id
			WHERE l.offer_id = $1
			ORDER BY l.day_of_week ASC, l.id ASC;
		`
		rows, err := tx.Query(txCtx, query, offerID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var loc promo.SpecialOfferLocation
			if err := rows.Scan(
				&loc.ID, &loc.OfferID, &loc.CityID, &loc.CityName,
				&loc.AddressAr, &loc.AddressEn, &loc.Latitude, &loc.Longitude, &loc.Radius,
				&loc.DayOfWeek, &loc.TimeFrom, &loc.TimeTo,
				&loc.Status, &loc.AdminStatus, &loc.CreatedAt,
			); err != nil {
				return err
			}
			list = append(list, &loc)
		}
		return rows.Err()
	})
	return list, err
}

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
				SELECT p.id, p.offer_id, p.variant_id, COALESCE(pv.sku, ''), COALESCE(pv.price, 0),
				       p.custom_price, p.custom_discount_percentage, p.custom_discount_amount, p.custom_qty, p.created_at
				FROM promo.offer_products p
				LEFT JOIN catalog.product_variants pv ON pv.id = p.variant_id
				WHERE p.offer_id = $1;
			`, offer.ID)
			if pRows != nil {
				for pRows.Next() {
					var p promo.SpecialOfferProduct
					if err := pRows.Scan(
						&p.ID, &p.OfferID, &p.VariantID, &p.VariantName, &p.OriginalPrice,
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
