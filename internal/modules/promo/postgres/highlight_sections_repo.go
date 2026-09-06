package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/storage"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

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

// ExpirePromotionsAndCollectMedia marks expired ads, offers, and sponsorship requests as expired/inactive,
// clears their media URLs in the database, and returns all media URLs to be purged from storage.
func (r *Repository) ExpirePromotionsAndCollectMedia(ctx context.Context) ([]string, int64, error) {
	var (
		mediaURLs    []string
		totalExpired int64
	)

	err := r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		// 1. Collect and expire Ads
		adRows, err := tx.Query(txCtx, `
			SELECT id, media_url, image_url, thumbnail_url, pending_changes
			FROM promo.ads
			WHERE expires_at < now()
			  AND (is_active = true OR media_url <> '' OR image_url <> '' OR thumbnail_url <> '' OR pending_changes IS NOT NULL);
		`)
		if err != nil {
			return fmt.Errorf("query expired ads: %w", err)
		}
		defer adRows.Close()

		var adIDs []int64
		for adRows.Next() {
			var (
				id                 int64
				mediaURL           string
				imageURL           string
				thumbnailURL       *string
				pendingChangesJSON []byte
			)
			if err := adRows.Scan(&id, &mediaURL, &imageURL, &thumbnailURL, &pendingChangesJSON); err != nil {
				return fmt.Errorf("scan expired ad: %w", err)
			}
			adIDs = append(adIDs, id)
			if m := strings.TrimSpace(mediaURL); m != "" {
				mediaURLs = append(mediaURLs, m)
			}
			if img := strings.TrimSpace(imageURL); img != "" {
				mediaURLs = append(mediaURLs, img)
			}
			if thumbnailURL != nil && strings.TrimSpace(*thumbnailURL) != "" {
				mediaURLs = append(mediaURLs, strings.TrimSpace(*thumbnailURL))
			}
			if len(pendingChangesJSON) > 0 && string(pendingChangesJSON) != "null" {
				var pc promo.AdPendingChanges
				if json.Unmarshal(pendingChangesJSON, &pc) == nil {
					if m := strings.TrimSpace(pc.MediaURL); m != "" {
						mediaURLs = append(mediaURLs, m)
					}
					if t := strings.TrimSpace(pc.ThumbnailURL); t != "" {
						mediaURLs = append(mediaURLs, t)
					}
				}
			}
		}
		if err := adRows.Err(); err != nil {
			return err
		}

		if len(adIDs) > 0 {
			tagAds, err := tx.Exec(txCtx, `
				UPDATE promo.ads
				SET is_active = false,
				    media_url = '',
				    image_url = '',
				    thumbnail_url = '',
				    pending_changes = NULL,
				    updated_at = now()
				WHERE id = ANY($1);
			`, adIDs)
			if err != nil {
				return fmt.Errorf("update expired ads: %w", err)
			}
			totalExpired += tagAds.RowsAffected()
		}

		// 2. Collect and expire Sponsorship Requests
		srRows, err := tx.Query(txCtx, `
			SELECT id, item_type, item_id
			FROM promo.sponsorship_requests
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return fmt.Errorf("query expired sponsorship requests: %w", err)
		}
		defer srRows.Close()

		var (
			srIDs    []int64
			offerIDs []int64
		)
		for srRows.Next() {
			var (
				id       int64
				itemType string
				itemID   int64
			)
			if err := srRows.Scan(&id, &itemType, &itemID); err != nil {
				return fmt.Errorf("scan expired sponsorship request: %w", err)
			}
			srIDs = append(srIDs, id)
			if itemType == "offer" && itemID > 0 {
				offerIDs = append(offerIDs, itemID)
			}
		}
		if err := srRows.Err(); err != nil {
			return err
		}

		if len(srIDs) > 0 {
			tagSR, err := tx.Exec(txCtx, `
				UPDATE promo.sponsorship_requests
				SET status = 'expired', updated_at = now()
				WHERE id = ANY($1);
			`, srIDs)
			if err != nil {
				return fmt.Errorf("update expired sponsorship requests: %w", err)
			}
			totalExpired += tagSR.RowsAffected()
		}

		// 3. Expire offer sponsorships and purchases
		tagSponsors, err := tx.Exec(txCtx, `
			UPDATE promo.offer_sponsorships
			SET status = 'expired'
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return fmt.Errorf("expire offer sponsorships: %w", err)
		}
		totalExpired += tagSponsors.RowsAffected()

		tagPurchases, err := tx.Exec(txCtx, `
			UPDATE promo.sponsorship_purchases
			SET status = 'expired', updated_at = now()
			WHERE status = 'active' AND expires_at < now();
		`)
		if err != nil {
			return fmt.Errorf("expire sponsorship purchases: %w", err)
		}
		totalExpired += tagPurchases.RowsAffected()

		// 4. Collect and expire Offers (both directly expired offers and offers from expired sponsorship requests)
		offerRows, err := tx.Query(txCtx, `
			SELECT id, image
			FROM promo.offers
			WHERE (is_active = true AND expires_at < now())
			   OR (id = ANY($1) AND image <> '');
		`, offerIDs)
		if err != nil {
			return fmt.Errorf("query expiring offers: %w", err)
		}
		defer offerRows.Close()

		var expiringOfferIDs []int64
		for offerRows.Next() {
			var (
				id    int64
				image string
			)
			if err := offerRows.Scan(&id, &image); err != nil {
				return fmt.Errorf("scan expiring offer: %w", err)
			}
			expiringOfferIDs = append(expiringOfferIDs, id)
			if img := strings.TrimSpace(image); img != "" {
				mediaURLs = append(mediaURLs, img)
			}
		}
		if err := offerRows.Err(); err != nil {
			return err
		}

		if len(expiringOfferIDs) > 0 {
			tagOffers, err := tx.Exec(txCtx, `
				UPDATE promo.offers
				SET is_active = false, image = '', updated_at = now()
				WHERE id = ANY($1);
			`, expiringOfferIDs)
			if err != nil {
				return fmt.Errorf("update expired offers: %w", err)
			}
			totalExpired += tagOffers.RowsAffected()
		}

		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	// Deduplicate media URLs
	seen := make(map[string]struct{}, len(mediaURLs))
	deduped := make([]string, 0, len(mediaURLs))
	for _, u := range mediaURLs {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; !ok {
			seen[u] = struct{}{}
			deduped = append(deduped, u)
		}
	}

	return deduped, totalExpired, nil
}

// ExpirePromotions marks expired offers, sponsorships, and ads as inactive and purges their media.
func (r *Repository) ExpirePromotions(ctx context.Context) (int64, error) {
	urls, count, err := r.ExpirePromotionsAndCollectMedia(ctx)
	if err != nil {
		return 0, err
	}
	for _, u := range urls {
		_ = storage.DeleteUploadedMedia(ctx, u, nil)
	}
	return count, nil
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
				COALESCE(NULLIF($12, ''), 'pending'), COALESCE($13, ''), 'special'
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

		// A variant that is not this vendor's, or no longer exists, is skipped
		// rather than aborting the offer with a NOT NULL violation on product_id.
		if _, err := insertSpecialOfferProducts(txCtx, tx, o.ID, o.OrganizationID, o.Products); err != nil {
			return err
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
			       o.admin_status, COALESCE(o.admin_notes, ''), COALESCE(o.image, ''),
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
			&o.StartDate, &o.EndDate, &o.Status, &o.AdminStatus, &o.AdminNotes, &o.Image,
			&o.CreatedAt, &o.UpdatedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("special_offer")
			}
			return err
		}

		prods, err := loadSpecialOfferProducts(txCtx, tx, id)
		if err != nil {
			return err
		}
		o.Products = prods

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

// ListSpecialOffersByOrg returns all special offers for an organization with their products.
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
			       o.admin_status, COALESCE(o.admin_notes, ''), COALESCE(o.image, ''),
			       o.created_at, o.updated_at
			FROM promo.offers o
			LEFT JOIN org.branches b ON b.id = o.branch_id
			WHERE o.organization_id = $1
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

		// Load products for all retrieved offers
		for _, o := range list {
			prods, err := loadSpecialOfferProducts(txCtx, tx, o.ID)
			if err != nil {
				return err
			}
			o.Products = prods
		}

		return nil
	})
	return list, err
}

// DeleteSpecialOffer deletes a special offer (cascades to its products and
// location covers through the offers family) and deletes its media file.
func (r *Repository) DeleteSpecialOffer(ctx context.Context, id, orgID int64) error {
	var image string
	_ = r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(txCtx, `SELECT image FROM promo.offers WHERE id = $1 AND organization_id = $2;`, id, orgID).Scan(&image)
	})

	err := r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(txCtx, `DELETE FROM promo.offers WHERE id = $1 AND organization_id = $2;`, id, orgID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("special_offer")
		}
		return nil
	})
	if err == nil && image != "" {
		_ = storage.DeleteUploadedMedia(ctx, image, nil)
	}
	return err
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
			ON CONFLICT (offer_id, city_id) DO UPDATE SET
				address_ar = EXCLUDED.address_ar,
				address_en = EXCLUDED.address_en,
				latitude = EXCLUDED.latitude,
				longitude = EXCLUDED.longitude,
				radius_meters = EXCLUDED.radius_meters,
				day_of_week = EXCLUDED.day_of_week,
				time_from = EXCLUDED.time_from,
				time_to = EXCLUDED.time_to,
				status = EXCLUDED.status,
				admin_status = EXCLUDED.admin_status
			RETURNING id, created_at;
		`
		return tx.QueryRow(txCtx, query,
			loc.OfferID, loc.CityID, loc.AddressAr, loc.AddressEn, loc.Latitude, loc.Longitude,
			loc.Radius, loc.DayOfWeek, loc.TimeFrom, loc.TimeTo, loc.Status, loc.AdminStatus,
		).Scan(&loc.ID, &loc.CreatedAt)
	})
}

// DeleteSpecialOfferLocation removes one coverage area for an offer.
func (r *Repository) DeleteSpecialOfferLocation(ctx context.Context, id, offerID, orgID int64) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			DELETE FROM promo.offer_location_covers
			WHERE id = $1 AND offer_id = $2
			  AND ($3 = 0 OR organization_id = $3);
		`
		ct, err := tx.Exec(txCtx, query, id, offerID, orgID)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return apperr.NotFound("special_offer_location")
		}
		return nil
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
