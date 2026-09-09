package postgres

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (r *Repository) readVendorProjection(
	ctx context.Context, actor authctx.Actor, q assistant.ProjectionQuery,
) (assistant.Page[assistant.ProjectionRow], error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return assistant.Page[assistant.ProjectionRow]{}, err
	}
	args := projectionArgs(orgID, q)
	switch q.Kind {
	case assistant.ProjectionVariantDetails:
		return r.readProjectionRows(ctx, actor, `
			SELECT v.id, jsonb_build_object(
				'product', `+nameExpr("p.name")+`, 'variant', COALESCE(`+nameExpr("v.name")+`,''),
				'sku', COALESCE(v.sku,''), 'price', v.price::text, 'discount', v.discount::text,
				'status', v.status, 'quota_limit', v.quota_limit,
				'stock', COALESCE((SELECT SUM(s.quantity) FROM inventory.stocks s WHERE s.product_variant_id=v.id AND s.deleted_at IS NULL),0))
			  FROM catalog.product_variants v JOIN catalog.products p ON p.id=v.product_id
			 WHERE v.id = $1 AND v.organization_id = $2 AND v.deleted_at IS NULL`, []any{q.ID, orgID}, 1, 0, false)
	case assistant.ProjectionStockByWarehouse:
		return r.readProjectionRows(ctx, actor, `
			SELECT s.warehouse_id, jsonb_build_object(
				'warehouse', COALESCE(w.name,''), 'code', COALESCE(w.code,''),
				'product', `+nameExpr("p.name")+`, 'variant', COALESCE(`+nameExpr("v.name")+`,''),
				'quantity', s.quantity, 'minimum', s.min_threshold)
			  FROM inventory.stocks s
			  JOIN inventory.warehouses w ON w.id=s.warehouse_id
			  JOIN catalog.products p ON p.id=s.product_id
			  LEFT JOIN catalog.product_variants v ON v.id=s.product_variant_id
			 WHERE s.organization_id=$1 AND s.deleted_at IS NULL
			   AND ($2='' OR w.name ILIKE '%' || $2 || '%' OR `+nameExpr("p.name")+` ILIKE '%' || $2 || '%')
			   AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY s.quantity ASC, s.id ASC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionWarehouseTransfers:
		return r.readProjectionRows(ctx, actor, `
			SELECT wt.id, jsonb_build_object(
				'from_warehouse', COALESCE(fw.name,''), 'to_warehouse', COALESCE(tw.name,''),
				'product', `+nameExpr("p.name")+`, 'quantity', wt.quantity,
				'status', wt.status, 'notes', COALESCE(wt.notes,''), 'created_at', wt.created_at)
			  FROM inventory.warehouse_transfers wt
			  LEFT JOIN inventory.warehouses fw ON fw.id=wt.from_warehouse_id
			  LEFT JOIN inventory.warehouses tw ON tw.id=wt.to_warehouse_id
			  JOIN catalog.products p ON p.id=wt.product_id
			 WHERE wt.organization_id=$1
			   AND ($2::text = '' OR TRUE)
			   AND ($3='' OR wt.status=$3)
			   AND ($4::timestamptz IS NULL OR wt.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR wt.created_at <= $5)
			 ORDER BY wt.created_at DESC, wt.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionQuotaReport:
		return r.readProjectionRows(ctx, actor, `
			SELECT v.id, jsonb_build_object(
				'product', `+nameExpr("p.name")+`, 'branch', COALESCE(`+nameExpr("b.name")+`,''),
				'quota_limit', v.quota_limit,
				'used', COALESCE((SELECT SUM(ol.quantity) FROM commerce.order_lines ol
					JOIN commerce.orders qo ON qo.id=ol.order_id
					WHERE ol.product_variant_id=v.id AND ol.organization_id=v.organization_id
					  AND qo.deleted_at IS NULL AND qo.status NOT IN ('cancelled','failed','returned','refunded')),0),
				'remaining', GREATEST(v.quota_limit-COALESCE((SELECT SUM(ol.quantity) FROM commerce.order_lines ol
					JOIN commerce.orders qo ON qo.id=ol.order_id
					WHERE ol.product_variant_id=v.id AND ol.organization_id=v.organization_id
					  AND qo.deleted_at IS NULL AND qo.status NOT IN ('cancelled','failed','returned','refunded')),0),0))
			  FROM catalog.product_variants v JOIN catalog.products p ON p.id=v.product_id
			  LEFT JOIN org.branches b ON b.id=v.branch_id
			 WHERE v.organization_id=$1 AND v.quota_limit IS NOT NULL AND v.quota_limit > 0
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY v.quota_limit ASC, v.id ASC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionCoverageReport:
		return r.readProjectionRows(ctx, actor, `
			SELECT wc.id, jsonb_build_object(
				'branch', COALESCE(`+nameExpr("b.name")+`,''), 'day', wc.day_of_week,
				'from', wc.coverage_from, 'to', wc.coverage_to,
				'distance_meters', wc.distance_meters, 'address', COALESCE(wc.address,''),
				'active', wc.is_active)
			  FROM workflow.weekly_coverages wc LEFT JOIN org.branches b ON b.id=wc.branch_id
			 WHERE wc.organization_id=$1 AND ($2='' OR wc.address ILIKE '%' || $2 || '%' OR `+nameExpr("b.name")+` ILIKE '%' || $2 || '%')
			   AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY wc.day_of_week, wc.coverage_from LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionOrdersByStatus:
		return r.readProjectionRows(ctx, actor, `
			SELECT ROW_NUMBER() OVER (ORDER BY sh.status)::bigint, jsonb_build_object(
				'status', sh.status, 'orders', COUNT(*), 'total', COALESCE(SUM(sh.total_amount),0)::text)
			  FROM commerce.order_shipments sh
			 WHERE sh.organization_id=$1 AND ($4::timestamptz IS NULL OR sh.created_at >= $4)
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE)
			   AND ($5::timestamptz IS NULL OR sh.created_at <= $5)
			 GROUP BY sh.status ORDER BY sh.status LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionRevenueByPeriod:
		return r.readProjectionRows(ctx, actor, `
			SELECT ROW_NUMBER() OVER (ORDER BY date_trunc('month', sh.created_at))::bigint,
				jsonb_build_object('period', to_char(date_trunc('month', sh.created_at),'YYYY-MM'),
				'revenue', COALESCE(SUM(sh.total_amount),0)::text, 'orders', COUNT(*))
			  FROM commerce.order_shipments sh
			 WHERE sh.organization_id=$1 AND ($4::timestamptz IS NULL OR sh.created_at >= $4)
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE)
			   AND ($5::timestamptz IS NULL OR sh.created_at <= $5)
			 GROUP BY date_trunc('month', sh.created_at) ORDER BY 1 DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionRevenueByProduct:
		return r.readProjectionRows(ctx, actor, `
			SELECT COALESCE(l.product_id, 0), jsonb_build_object('product', `+nameExpr("l.product_name")+`,
				'quantity', SUM(l.quantity), 'revenue', COALESCE(SUM(l.total_price),0)::text,
				'orders', COUNT(DISTINCT l.shipment_id))
			  FROM commerce.order_lines l JOIN commerce.order_shipments sh ON sh.id=l.shipment_id
			 WHERE l.organization_id=$1 AND ($2='' OR `+nameExpr("l.product_name")+` ILIKE '%' || $2 || '%')
			   AND ($3::text = '' OR TRUE)
			   AND ($4::timestamptz IS NULL OR sh.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR sh.created_at <= $5)
			 GROUP BY l.product_id, l.product_name ORDER BY SUM(l.total_price) DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionCustomers:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.organization_id, jsonb_build_object('organization', `+nameExpr("buyer.name")+`,
				'orders', COUNT(DISTINCT o.id), 'revenue', COALESCE(SUM(sh.total_amount),0)::text,
				'last_order_at', MAX(sh.created_at))
			  FROM commerce.order_shipments sh JOIN commerce.orders o ON o.id=sh.order_id
			  JOIN org.organizations buyer ON buyer.id=o.organization_id
			 WHERE sh.organization_id=$1 AND ($2='' OR `+nameExpr("buyer.name")+` ILIKE '%' || $2 || '%')
			   AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 GROUP BY o.organization_id, buyer.name ORDER BY SUM(sh.total_amount) DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionImportRuns:
		return r.readProjectionRows(ctx, actor, `
			SELECT ci.id, jsonb_build_object('filename', ci.filename, 'phase', ci.phase,
				'status', ci.phase, 'total_rows', ci.total_rows, 'inserted_rows', ci.inserted_rows,
				'updated_rows', ci.updated_rows, 'error_rows', ci.error_rows, 'created_at', ci.created_at)
			  FROM ingest.catalog_imports ci WHERE ci.organization_id=$1
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE)
			   AND ($4::timestamptz IS NULL OR ci.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR ci.created_at <= $5)
			 ORDER BY ci.created_at DESC, ci.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionImportRunDetails:
		return r.readProjectionRows(ctx, actor, `
			SELECT ci.id, jsonb_build_object('filename', ci.filename, 'phase', ci.phase,
				'total_rows', ci.total_rows, 'inserted_rows', ci.inserted_rows,
				'updated_rows', ci.updated_rows, 'skipped_rows', ci.skipped_rows,
				'error_rows', ci.error_rows, 'matched_rows', ci.matched_rows,
				'review_rows', ci.review_rows, 'unmatched_rows', ci.unmatched_rows,
				'error_message', COALESCE(ci.error_message,''))
			  FROM ingest.catalog_imports ci WHERE ci.id=$1 AND ci.organization_id=$2`, []any{q.ID, orgID}, 1, 0, false)
	case assistant.ProjectionOffersPerformance:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object('offer', `+nameExpr("o.title")+`,
				'impressions', COALESCE((SELECT COUNT(*) FROM promo.ad_impressions ai WHERE ai.ad_id=o.id),0),
				'clicks', COALESCE((SELECT COUNT(*) FROM promo.offer_clicks oc WHERE oc.offer_id=o.id),0),
				'conversions', COALESCE((SELECT COUNT(*) FROM commerce.cart_items ci WHERE ci.offer_id=o.id),0),
				'active', o.is_active, 'expires_at', o.expires_at)
			  FROM promo.offers o WHERE o.organization_id=$1 AND o.deleted_at IS NULL
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY o.clicks_count DESC, o.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionSponsorshipStatus:
		return r.readProjectionRows(ctx, actor, `
			SELECT sp.id, jsonb_build_object('status', sp.status, 'credits_total', sp.credits_total,
				'credits_used', sp.credits_used, 'remaining', sp.credits_total-sp.credits_used,
				'amount', sp.amount::text, 'starts_at', sp.starts_at, 'expires_at', sp.expires_at,
				'package', COALESCE(`+nameExpr("op.name")+`,''))
			  FROM promo.sponsorship_purchases sp LEFT JOIN promo.offer_packages op ON op.id=sp.package_id
			 WHERE sp.organization_id=$1 AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY sp.created_at DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionTeam:
		return r.readProjectionRows(ctx, actor, `
			SELECT m.user_id, jsonb_build_object('member', COALESCE(`+nameExpr("u.name")+`,u.email,''),
				'email', u.email, 'role', COALESCE(m.role_key,''), 'status', m.status,
				'job_title', COALESCE(m.job_title,''), 'joined_at', m.joined_at)
			  FROM org.members m JOIN identity.users u ON u.id=m.user_id
			 WHERE m.organization_id=$1 AND ($2='' OR `+nameExpr("u.name")+` ILIKE '%' || $2 || '%' OR u.email ILIKE '%' || $2 || '%')
			   AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY m.created_at DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionReviews:
		return r.readProjectionRows(ctx, actor, `
			SELECT r.id, jsonb_build_object('rating', r.rating, 'title', COALESCE(r.title,''),
				'text', COALESCE(r.review_text,''), 'status', r.status, 'created_at', r.created_at,
				'reviewer', COALESCE(`+nameExpr("u.name")+`,''))
			  FROM org.organization_reviews r LEFT JOIN identity.users u ON u.id=r.user_id
			 WHERE r.organization_id=$1 AND r.deleted_at IS NULL
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY r.created_at DESC, r.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	default:
		return assistant.Page[assistant.ProjectionRow]{}, fmt.Errorf("assistant: unsupported vendor projection %q", q.Kind)
	}
}
