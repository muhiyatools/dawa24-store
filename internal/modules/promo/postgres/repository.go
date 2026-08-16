package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
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
				min_order_value, starts_at, expires_at, is_active
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, public_id, created_at, updated_at;
		`
		err := tx.QueryRow(txCtx, query,
			o.OrganizationID, o.Title, o.Description, string(o.DiscountType),
			o.DiscountValue, o.MinOrderValue, o.StartsAt, o.ExpiresAt, o.IsActive,
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
	var o promo.Offer
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, title, description, discount_type,
			       discount_value, min_order_value, starts_at, expires_at, is_active,
			       views_count, clicks_count, created_at, updated_at, deleted_at
			FROM promo.offers
			WHERE id = $1 AND deleted_at IS NULL;
		`
		var discType string
		err := tx.QueryRow(txCtx, query, id).Scan(
			&o.ID, &o.PublicID, &o.OrganizationID, &o.Title, &o.Description,
			&discType, &o.DiscountValue, &o.MinOrderValue, &o.StartsAt, &o.ExpiresAt,
			&o.IsActive, &o.ViewsCount, &o.ClicksCount, &o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
		)
		if err != nil {
			if database.IsNotFound(err) {
				return apperr.NotFound("offer")
			}
			return err
		}
		o.DiscountType = promo.DiscountType(discType)

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
			o.ProductIDs = append(o.ProductIDs, pID)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ListActiveOffers returns all currently active offers across all vendors.
func (r *Repository) ListActiveOffers(ctx context.Context, limit, offset int) ([]*promo.Offer, error) {
	var offers []*promo.Offer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT id, public_id, organization_id, title, description, discount_type,
			       discount_value, min_order_value, starts_at, expires_at, is_active,
			       views_count, clicks_count, created_at, updated_at, deleted_at
			FROM promo.offers
			WHERE is_active = true AND starts_at <= now() AND expires_at >= now() AND deleted_at IS NULL
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
			var o promo.Offer
			var discType string
			if err := rows.Scan(
				&o.ID, &o.PublicID, &o.OrganizationID, &o.Title, &o.Description,
				&discType, &o.DiscountValue, &o.MinOrderValue, &o.StartsAt, &o.ExpiresAt,
				&o.IsActive, &o.ViewsCount, &o.ClicksCount, &o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
			); err != nil {
				return err
			}
			o.DiscountType = promo.DiscountType(discType)
			offers = append(offers, &o)
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

// CreateHighlightSection creates a homepage curated section.
func (r *Repository) CreateHighlightSection(ctx context.Context, h *promo.HighlightSection) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.highlight_sections (title, slug, display_order, is_active)
			VALUES ($1, $2, $3, $4)
			RETURNING id, public_id, created_at;
		`
		return tx.QueryRow(txCtx, query, h.Title, h.Slug, h.DisplayOrder, h.IsActive).
			Scan(&h.ID, &h.PublicID, &h.CreatedAt)
	})
}

// ListHighlightSections returns all active highlight sections.
func (r *Repository) ListHighlightSections(ctx context.Context) ([]*promo.HighlightSection, error) {
	var list []*promo.HighlightSection
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT id, public_id, title, slug, display_order, is_active, created_at FROM promo.highlight_sections WHERE is_active = true ORDER BY display_order ASC;`
		rows, err := tx.Query(txCtx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var h promo.HighlightSection
			if err := rows.Scan(&h.ID, &h.PublicID, &h.Title, &h.Slug, &h.DisplayOrder, &h.IsActive, &h.CreatedAt); err != nil {
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
