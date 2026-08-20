package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/money"
)

// ListOffersForProduct returns every approved, currently running offer that
// sells the product, newest first. Customers may buy from any vendor, so the
// read is system-scoped; only approved offers are commerce-visible (062).
func (r *Repository) ListOffersForProduct(ctx context.Context, productID int64) ([]*promo.OfferProductWithOffer, error) {
	var matches []*promo.OfferProductWithOffer
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		query := `
			SELECT op.id, op.offer_id, op.product_id, op.variant_id,
			       op.custom_price, op.custom_discount_percentage, op.custom_discount_amount,
			       op.custom_qty, op.max_qty_per_order, op.created_at,
			       o.id, o.public_id, o.organization_id, o.branch_id,
			       o.title, o.description, o.discount_type, o.discount_value,
			       o.min_order_amount,
			       o.admin_status, o.admin_notes, o.approved_at, o.approved_by,
			       o.rejected_at, o.rejected_by, o.starts_at, o.expires_at, o.is_active,
			       o.views_count, o.clicks_count, o.created_at, o.updated_at, o.deleted_at
			FROM promo.offer_products op
			JOIN promo.offers o ON o.id = op.offer_id
			WHERE op.product_id = $1
			  AND o.deleted_at IS NULL
			  AND o.is_active = true
			  AND o.admin_status = 'approved'
			  AND o.starts_at <= now() AND o.expires_at >= now()
			ORDER BY o.id DESC;
		`
		rows, err := tx.Query(txCtx, query, productID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			item, err := scanOfferProductWithOffer(rows)
			if err != nil {
				return err
			}
			matches = append(matches, item)
		}
		return rows.Err()
	})
	return matches, err
}

// offerColumns is the canonical promo.offers projection; scanOffer reads it in
// exactly this order. Every offer SELECT (storefront, dashboard, moderation,
// visibility) should reuse it so the column order lives in one place.
const offerColumns = `id, public_id, organization_id, branch_id, title, description,
	discount_type, discount_value, min_order_amount,
	admin_status, admin_notes, approved_at, approved_by,
	rejected_at, rejected_by, starts_at, expires_at, is_active,
	views_count, clicks_count, created_at, updated_at, deleted_at`

// scanOffer maps the canonical promo.offers projection onto the domain type.
// The SELECT column order must match: id, public_id, organization_id, branch_id,
// title, description, discount_type, discount_value, min_order_amount,
// admin_status, admin_notes, approved_at, approved_by, rejected_at, rejected_by,
// starts_at, expires_at, is_active, views_count, clicks_count, created_at,
// updated_at, deleted_at.
func scanOffer(row pgx.Row) (*promo.Offer, error) {
	var (
		o          promo.Offer
		discType   string
		branchID   *int64
		approvedAt *time.Time
		approvedBy *int64
		rejectedAt *time.Time
		rejectedBy *int64
	)
	err := row.Scan(
		&o.ID, &o.PublicID, &o.OrganizationID, &branchID, &o.Title, &o.Description,
		&discType, &o.DiscountValue, &o.MinOrderAmount,
		&o.AdminStatus, &o.AdminNotes, &approvedAt, &approvedBy,
		&rejectedAt, &rejectedBy,
		&o.StartsAt, &o.ExpiresAt, &o.IsActive,
		&o.ViewsCount, &o.ClicksCount, &o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
	)
	if err != nil {
		return nil, err
	}
	o.DiscountType = promo.DiscountType(discType)
	o.BranchID = branchID
	o.ApprovedAt = approvedAt
	o.ApprovedBy = derefInt64(approvedBy)
	o.RejectedAt = rejectedAt
	o.RejectedBy = derefInt64(rejectedBy)
	if o.AdminStatus == "" {
		o.AdminStatus = "pending"
	}
	return &o, nil
}

// scanOfferProductWithOffer maps one joined row onto the domain types.
func scanOfferProductWithOffer(row pgx.Row) (*promo.OfferProductWithOffer, error) {
	var (
		out            promo.OfferProductWithOffer
		op             promo.OfferProduct
		o              promo.Offer
		discType       string
		branchID       *int64
		approvedAt     *time.Time
		approvedBy     *int64
		rejectedAt     *time.Time
		rejectedBy     *int64
		variantID      *int64
		customPrice    *money.Amount
		customPct      *float64
		customAmt      *money.Amount
		maxQtyPerOrder *int
	)
	out.Product = &op
	out.Offer = &o

	err := row.Scan(
		&op.ID, &op.OfferID, &op.ProductID, &variantID,
		&customPrice, &customPct, &customAmt,
		&op.CustomQty, &maxQtyPerOrder, &op.CreatedAt,
		&o.ID, &o.PublicID, &o.OrganizationID, &branchID,
		&o.Title, &o.Description, &discType, &o.DiscountValue,
		&o.MinOrderAmount,
		&o.AdminStatus, &o.AdminNotes, &approvedAt, &approvedBy,
		&rejectedAt, &rejectedBy, &o.StartsAt, &o.ExpiresAt, &o.IsActive,
		&o.ViewsCount, &o.ClicksCount, &o.CreatedAt, &o.UpdatedAt, &o.DeletedAt,
	)
	if err != nil {
		return nil, err
	}

	op.VariantID = variantID
	op.CustomPrice = customPrice
	op.CustomDiscountPercent = customPct
	op.CustomDiscountAmount = customAmt
	op.MaxQtyPerOrder = maxQtyPerOrder
	if op.CustomQty <= 0 {
		op.CustomQty = 1
	}

	o.DiscountType = promo.DiscountType(discType)
	o.BranchID = branchID
	o.ApprovedAt = approvedAt
	o.ApprovedBy = derefInt64(approvedBy)
	o.RejectedAt = rejectedAt
	o.RejectedBy = derefInt64(rejectedBy)
	if o.AdminStatus == "" {
		o.AdminStatus = "pending"
	}
	return &out, nil
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
