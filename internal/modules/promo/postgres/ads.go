package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"
)

const adColumns = `
	id, public_id, organization_id, title, title_ar, title_en, ad_text_ar, ad_text_en,
	image_url, media_type, media_url, thumbnail_url, target_url,
	click_target_type, click_target_id, position, is_active, admin_status, admin_notes,
	reviewed_by, reviewed_at, ad_plan_id, duration_days, starts_at, expires_at,
	impressions, clicks, created_at, updated_at`

// CreateAd inserts a new advertisement.
func (r *Repository) CreateAd(ctx context.Context, a *promo.Ad) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			INSERT INTO promo.ads (
				organization_id, title, title_ar, title_en, ad_text_ar, ad_text_en,
				image_url, media_type, media_url, thumbnail_url, target_url,
				click_target_type, click_target_id, position, is_active, admin_status,
				ad_plan_id, duration_days, starts_at, expires_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
			RETURNING id, public_id, created_at, updated_at;
		`
		if a.AdminStatus == "" {
			a.AdminStatus = promo.AdminPending
		}
		if a.MediaType == "" {
			a.MediaType = promo.MediaImage
		}
		if a.ClickTargetType == "" {
			a.ClickTargetType = promo.ClickTargetVendor
		}
		if a.Position == "" {
			a.Position = "home_banner"
		}
		if a.DurationDays <= 0 {
			a.DurationDays = 30
		}
		if a.ExpiresAt.IsZero() {
			a.ExpiresAt = time.Now().UTC().Add(time.Duration(a.DurationDays) * 24 * time.Hour)
		}
		if a.StartsAt.IsZero() {
			a.StartsAt = time.Now().UTC()
		}
		imageURL := a.ImageURL
		if imageURL == "" && a.MediaURL != "" {
			imageURL = a.MediaURL
		}
		return tx.QueryRow(txCtx, query,
			a.OrganizationID, a.Title, a.TitleAr, a.TitleEn, a.AdTextAr, a.AdTextEn,
			imageURL, string(a.MediaType), a.MediaURL, a.ThumbnailURL, a.TargetURL,
			string(a.ClickTargetType), a.ClickTargetID, a.Position, a.IsActive, string(a.AdminStatus),
			a.AdPlanID, a.DurationDays, a.StartsAt, a.ExpiresAt,
		).Scan(&a.ID, &a.PublicID, &a.CreatedAt, &a.UpdatedAt)
	})
}

// UpdateAd updates an existing advertisement.
func (r *Repository) UpdateAd(ctx context.Context, a *promo.Ad) error {
	return r.db.InTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			UPDATE promo.ads SET
				title = $2, title_ar = $3, title_en = $4, ad_text_ar = $5, ad_text_en = $6,
				image_url = $7, media_type = $8, media_url = $9, thumbnail_url = $10, target_url = $11,
				click_target_type = $12, click_target_id = $13, position = $14, is_active = $15,
				duration_days = $16, starts_at = $17, expires_at = $18, updated_at = now()
			WHERE id = $1;
		`
		tag, err := tx.Exec(txCtx, query,
			a.ID, a.Title, a.TitleAr, a.TitleEn, a.AdTextAr, a.AdTextEn,
			a.ImageURL, string(a.MediaType), a.MediaURL, a.ThumbnailURL, a.TargetURL,
			string(a.ClickTargetType), a.ClickTargetID, a.Position, a.IsActive,
			a.DurationDays, a.StartsAt, a.ExpiresAt,
		)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("ad")
		}
		return nil
	})
}

// GetAdByID retrieves an ad by ID.
func (r *Repository) GetAdByID(ctx context.Context, id int64) (*promo.Ad, error) {
	var a promo.Ad
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		return scanAd(tx.QueryRow(txCtx, `SELECT `+adColumns+` FROM promo.ads WHERE id = $1;`, id), &a)
	})
	if err != nil {
		if database.IsNotFound(err) {
			return nil, apperr.NotFound("ad")
		}
		return nil, err
	}
	return &a, nil
}

// ListAdsByOrg returns ads for an organization.
func (r *Repository) ListAdsByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*promo.Ad, error) {
	return r.listAds(ctx, `WHERE organization_id = $1`, orgID, limit, offset)
}

// ListAllAds returns all ads for admin moderation.
func (r *Repository) ListAllAds(ctx context.Context, limit, offset int) ([]*promo.Ad, error) {
	return r.listAds(database.AsSystem(ctx), ``, int64(0), limit, offset)
}

func (r *Repository) listAds(ctx context.Context, where string, orgID int64, limit, offset int) ([]*promo.Ad, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list []*promo.Ad
	err := r.db.InReadTx(ctx, func(txCtx context.Context, tx pgx.Tx) error {
		query := `SELECT ` + adColumns + ` FROM promo.ads ` + where + ` ORDER BY created_at DESC LIMIT $` + adLimitParam(orgID) + ` OFFSET $` + adOffsetParam(orgID) + `;`
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
			var a promo.Ad
			if err := scanAd(rows, &a); err != nil {
				return err
			}
			list = append(list, &a)
		}
		return rows.Err()
	})
	return list, err
}

func adLimitParam(orgID int64) string {
	if orgID > 0 {
		return "2"
	}
	return "1"
}
func adOffsetParam(orgID int64) string {
	if orgID > 0 {
		return "3"
	}
	return "2"
}

// ListActiveAds returns approved, active display ads by screen position.
func (r *Repository) ListActiveAds(ctx context.Context, position string) ([]*promo.Ad, error) {
	var list []*promo.Ad
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT ` + adColumns + ` FROM promo.ads
			WHERE is_active = true AND admin_status = 'approved'
			  AND starts_at <= now() AND expires_at >= now()
			  AND ($1 = '' OR position = $1)
			ORDER BY created_at DESC;
		`
		rows, err := tx.Query(txCtx, query, position)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a promo.Ad
			if err := scanAd(rows, &a); err != nil {
				return err
			}
			a.CTR = promo.ComputeCTR(a.Impressions, a.Clicks)
			list = append(list, &a)
		}
		return rows.Err()
	})
	return list, err
}

// UpdateAdAdminStatus sets the admin approval state.
func (r *Repository) UpdateAdAdminStatus(ctx context.Context, id int64, status promo.AdminStatus, notes string, reviewerID int64) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		isActive := status == promo.AdminApproved
		tag, err := tx.Exec(txCtx, `
			UPDATE promo.ads
			SET admin_status = $1, admin_notes = $2, reviewed_by = $3, reviewed_at = now(),
			    is_active = $4, updated_at = now()
			WHERE id = $5;
		`, string(status), notes, reviewerID, isActive, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("ad")
		}
		return nil
	})
}

// RecordAdImpression logs an impression and increments the counter.
func (r *Repository) RecordAdImpression(ctx context.Context, adID int64, userID *int64, ip, ua string) error {
	return r.db.InTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(txCtx, `UPDATE promo.ads SET impressions = impressions + 1 WHERE id = $1;`, adID); err != nil {
			return err
		}
		_, err := tx.Exec(txCtx, `INSERT INTO promo.ad_impressions (ad_id, user_id, ip_address, user_agent) VALUES ($1, $2, $3, $4);`, adID, userID, ip, ua)
		return err
	})
}

func scanAd(row pgx.Row, a *promo.Ad) error {
	var (
		mediaType    string
		clickTarget  string
		adminStatus  string
	)
	err := row.Scan(
		&a.ID, &a.PublicID, &a.OrganizationID, &a.Title, &a.TitleAr, &a.TitleEn, &a.AdTextAr, &a.AdTextEn,
		&a.ImageURL, &mediaType, &a.MediaURL, &a.ThumbnailURL, &a.TargetURL,
		&clickTarget, &a.ClickTargetID, &a.Position, &a.IsActive, &adminStatus, &a.AdminNotes,
		&a.ReviewedBy, &a.ReviewedAt, &a.AdPlanID, &a.DurationDays, &a.StartsAt, &a.ExpiresAt,
		&a.Impressions, &a.Clicks, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return err
	}
	a.MediaType = promo.AdMediaType(mediaType)
	a.ClickTargetType = promo.AdClickTarget(clickTarget)
	a.AdminStatus = promo.AdminStatus(adminStatus)
	return nil
}
