// Canonical coverage visibility query — Rebuild V2 §3.2.
//
// A pharmacy browsing for one of its branches sees offers from vendor branches
// whose weekly coverage circle contains the pharmacy branch's coordinates, on
// the requested day. This query is the ONLY place that rule is expressed; every
// customer-facing listing calls ListOffersVisibleTo.
//
// Deviation from the plan: migration 050 shipped without the cube/
// earthdistance extensions, so the earth_box pre-filter is dropped and the
// Haversine platform.distance_meters() carries the whole check. The result
// ordering and semantics are unchanged.
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
)

// ListOffersVisibleTo returns the offers a pharmacy branch can actually buy:
// published by approved vendor branches whose weekly coverage for the given
// weekday contains the pharmacy branch's coordinates. Coordinates come from
// the caller (actor.BranchID / the pharmacy's main branch) — never from the
// request.
//
// The offer columns use the canonical scanOffer order; the two trailing
// columns (vendor branch id, distance in metres) are read afterwards.
func (r *Repository) ListOffersVisibleTo(ctx context.Context, latitude, longitude float64, dayOfWeek, limit, offset int) ([]*promo.VisibleOffer, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var offers []*promo.VisibleOffer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT o.id, o.public_id, o.organization_id, o.branch_id,
			       o.title, o.description, o.discount_type, o.discount_value,
			       o.min_order_amount,
			       o.admin_status, o.admin_notes, o.approved_at, o.approved_by,
			       o.rejected_at, o.rejected_by, o.starts_at, o.expires_at, o.is_active,
			       o.views_count, o.clicks_count, o.created_at, o.updated_at, o.deleted_at,
			       vb.id AS vendor_branch_id,
			       platform.distance_meters(wc.latitude, wc.longitude, $1::NUMERIC, $2::NUMERIC) AS metres
			FROM promo.offers o
			JOIN org.branches           vb ON vb.id = o.branch_id AND vb.deleted_at IS NULL
			JOIN org.organizations      vo ON vo.id = o.organization_id
			JOIN workflow.weekly_coverages wc
			       ON wc.branch_id   = vb.id
			      AND wc.day_of_week = $3
			      AND wc.is_active = true
			WHERE o.is_active = true
			  AND o.admin_status = 'approved'
			  AND o.deleted_at IS NULL
			  AND vo.status = 'approved'
			  AND vo.type   = 'vendor'
			  AND o.starts_at <= now() AND o.expires_at >= now()
			  AND wc.latitude IS NOT NULL AND wc.longitude IS NOT NULL
			  AND platform.distance_meters(wc.latitude, wc.longitude, $1::NUMERIC, $2::NUMERIC) <= wc.distance_meters
			ORDER BY vo.is_sponsored DESC, metres ASC, o.created_at DESC
			LIMIT $4 OFFSET $5;

		`
		rows, err := tx.Query(txCtx, query, latitude, longitude, dayOfWeek, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			visible, err := scanVisibleOffer(rows)
			if err != nil {
				return err
			}
			offers = append(offers, visible)
		}
		return rows.Err()
	})
	return offers, err
}

// scanVisibleOffer maps the canonical visibility projection. It mirrors
// scanOffer's assignment for the 24 offer columns, then reads the two
// visibility extras; keep the two SELECTs in lockstep.
func scanVisibleOffer(row pgx.Row) (*promo.VisibleOffer, error) {
	var (
		offer          promo.Offer
		discType       string
		branchID       *int64
		approvedAt     *time.Time
		approvedBy     *int64
		rejectedAt     *time.Time
		rejectedBy     *int64
		vendorBranchID int64
		metres         *int
	)
	err := row.Scan(
		&offer.ID, &offer.PublicID, &offer.OrganizationID, &branchID,
		&offer.Title, &offer.Description, &discType, &offer.DiscountValue,
		&offer.MinOrderAmount,
		&offer.AdminStatus, &offer.AdminNotes, &approvedAt, &approvedBy,
		&rejectedAt, &rejectedBy, &offer.StartsAt, &offer.ExpiresAt, &offer.IsActive,
		&offer.ViewsCount, &offer.ClicksCount, &offer.CreatedAt, &offer.UpdatedAt, &offer.DeletedAt,
		&vendorBranchID, &metres,
	)
	if err != nil {
		return nil, err
	}

	offer.DiscountType = promo.DiscountType(discType)
	offer.BranchID = branchID
	offer.ApprovedAt = approvedAt
	offer.ApprovedBy = derefInt64(approvedBy)
	offer.RejectedAt = rejectedAt
	offer.RejectedBy = derefInt64(rejectedBy)
	if offer.AdminStatus == "" {
		offer.AdminStatus = "pending"
	}

	distance := 0
	if metres != nil {
		distance = *metres
	}
	return &promo.VisibleOffer{
		Offer:          &offer,
		VendorBranchID: vendorBranchID,
		Metres:         distance,
	}, nil
}