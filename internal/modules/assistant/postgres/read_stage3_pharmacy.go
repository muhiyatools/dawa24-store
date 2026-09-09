package postgres

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (r *Repository) readPharmacyProjection(
	ctx context.Context, actor authctx.Actor, q assistant.ProjectionQuery,
) (assistant.Page[assistant.ProjectionRow], error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return assistant.Page[assistant.ProjectionRow]{}, err
	}
	args := projectionArgs(orgID, q)
	switch q.Kind {
	case assistant.ProjectionReorderSuggestions:
		return r.readProjectionRows(ctx, actor, `
			SELECT COALESCE(l.product_id, 0), jsonb_build_object(
				'product', `+nameExpr("l.product_name")+`,
				'quantity', COALESCE(SUM(l.quantity),0),
				'orders', COUNT(DISTINCT o.id),
				'last_purchased_at', MAX(o.created_at))
			  FROM commerce.order_lines l
			  JOIN commerce.orders o ON o.id = l.order_id
			 WHERE o.organization_id = $1 AND o.deleted_at IS NULL
			   AND ($2 = '' OR `+nameExpr("l.product_name")+` ILIKE '%' || $2 || '%')
			   AND ($3::text = '' OR TRUE)
			   AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			   AND ($4::timestamptz IS NULL OR o.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR o.created_at <= $5)
			 GROUP BY l.product_id, l.product_name
			 ORDER BY MAX(o.created_at) ASC, SUM(l.quantity) DESC
			 LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionCatalogSearch:
		return r.readProjectionRows(ctx, actor, `
			SELECT v.product_id, jsonb_build_object(
				'product', `+nameExpr("p.name")+`, 'sku', COALESCE(v.sku,''),
				'supplier', COALESCE(`+nameExpr("org.name")+`,''),
				'price', v.price::text, 'discount', v.discount::text,
				'unit', COALESCE(v.unit,''), 'status', v.status)
			  FROM catalog.product_variants v
			  JOIN catalog.products p ON p.id = v.product_id
			  LEFT JOIN org.organizations org ON org.id = v.organization_id
			 WHERE v.organization_id <> $1 AND v.status = 'active' AND v.deleted_at IS NULL
			   AND p.deleted_at IS NULL AND p.status = 'active'
			   AND ($3::text = '' OR TRUE)
			   AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			   AND ($2 = '' OR `+nameExpr("p.name")+` ILIKE '%' || $2 || '%'
			        OR COALESCE(v.sku,'') ILIKE '%' || $2 || '%'
			        OR COALESCE(p.scientific_name,'') ILIKE '%' || $2 || '%')
			 ORDER BY (v.discount > 0) DESC, v.price ASC, v.id ASC
			 LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionOfferDetails:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object(
				'offer', `+nameExpr("o.title")+`, 'description', COALESCE(`+nameExpr("o.description")+`,''),
				'discount_type', o.discount_type, 'discount_value', o.discount_value::text,
				'starts_at', o.starts_at, 'expires_at', o.expires_at,
				'active', o.is_active, 'product_count',
				 (SELECT COUNT(*) FROM promo.offer_products op WHERE op.offer_id = o.id),
				'min_order_amount', COALESCE(o.min_order_amount,0)::text)
			  FROM promo.offers o
			 WHERE o.id = $1 AND o.deleted_at IS NULL`, []any{q.ID}, q.Limit, q.Offset, true)
	case assistant.ProjectionCartSummary:
		return r.readProjectionRows(ctx, actor, `
			SELECT c.id, jsonb_build_object(
				'item_count', COALESCE((SELECT SUM(ci.quantity) FROM commerce.cart_items ci WHERE ci.cart_id = c.id),0),
				'total', COALESCE((SELECT SUM(ci.quantity * ci.unit_price) FROM commerce.cart_items ci WHERE ci.cart_id = c.id),0)::text,
				'items', COALESCE((SELECT jsonb_agg(jsonb_build_object(
					'product', `+nameExpr("p.name")+`, 'quantity', ci.quantity, 'unit_price', ci.unit_price::text))
					FROM commerce.cart_items ci JOIN catalog.products p ON p.id = ci.product_id
					WHERE ci.cart_id = c.id), '[]'::jsonb))
			  FROM commerce.carts c
			 WHERE c.organization_id = $1 AND c.user_id = $2
			 ORDER BY c.updated_at DESC, c.id DESC LIMIT 1`, []any{orgID, actor.UserID}, 1, 0, false)
	case assistant.ProjectionPurchaseRequests:
		return r.readProjectionRows(ctx, actor, `
			SELECT pr.id, jsonb_build_object(
				'request_number', pr.request_number, 'status', pr.status,
				'items', pr.total_items, 'estimated_total', pr.estimated_total::text,
				'created_at', pr.created_at, 'vendor', COALESCE(`+nameExpr("org.name")+`,''))
			  FROM commerce.purchase_requests pr
			  LEFT JOIN org.organizations org ON org.id = pr.vendor_org_id
			 WHERE pr.organization_id = $1
			   AND ($2::text = '' OR TRUE)
			   AND ($3 = '' OR pr.status = $3)
			   AND ($4::timestamptz IS NULL OR pr.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR pr.created_at <= $5)
			 ORDER BY pr.created_at DESC, pr.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionInvoices:
		return r.readProjectionRows(ctx, actor, `
			SELECT i.id, jsonb_build_object(
				'invoice_number', i.invoice_number, 'status', i.status,
				'issue_date', i.issue_date, 'due_date', i.due_date,
				'total', i.total_amount::text, 'payment_method', COALESCE(i.payment_method,''))
			  FROM billing.invoices i
			 WHERE (i.organization_id = $1 OR i.customer_org_id = $1)
			   AND ($2::text = '' OR TRUE)
			   AND ($3 = '' OR i.status = $3)
			   AND ($4::timestamptz IS NULL OR i.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR i.created_at <= $5)
			 ORDER BY i.issue_date DESC, i.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionInvoiceDetails:
		return r.readProjectionRows(ctx, actor, `
			SELECT i.id, jsonb_build_object(
				'invoice_number', i.invoice_number, 'status', i.status,
				'issue_date', i.issue_date, 'due_date', i.due_date,
				'subtotal', i.subtotal::text, 'tax', i.tax_amount::text,
				'discount', i.discount_amount::text, 'total', i.total_amount::text,
				'lines', COALESCE((SELECT jsonb_agg(jsonb_build_object(
					'description', il.description, 'quantity', il.quantity,
					'unit_price', il.unit_price::text, 'total', il.total_price::text))
					FROM billing.invoice_lines il WHERE il.invoice_id = i.id), '[]'::jsonb))
			  FROM billing.invoices i
			 WHERE i.id = $1 AND (i.organization_id = $2 OR i.customer_org_id = $2)`, []any{q.ID, orgID}, 1, 0, false)
	case assistant.ProjectionPayments:
		return r.readProjectionRows(ctx, actor, `
			SELECT p.id, jsonb_build_object(
				'reference', COALESCE(p.reference_number,''), 'amount', p.amount::text,
				'method', p.method, 'status', p.status, 'paid_at', p.paid_at,
				'invoice', COALESCE(i.invoice_number,''))
			  FROM billing.payments p LEFT JOIN billing.invoices i ON i.id = p.invoice_id
			 WHERE p.organization_id = $1
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE)
			   AND ($4::timestamptz IS NULL OR p.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR p.created_at <= $5)
			 ORDER BY p.created_at DESC, p.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionSavingProducts:
		return r.readProjectionRows(ctx, actor, `
			SELECT sp.product_id, jsonb_build_object(
				'product', COALESCE(sp.name_product, `+nameExpr("p.name")+`),
				'sku', COALESCE(sp.sku,''), 'quantity', sp.qty, 'price', sp.price::text,
				'created_at', sp.created_at)
			  FROM catalog.saving_products sp LEFT JOIN catalog.products p ON p.id = sp.product_id
			 WHERE sp.organization_id = $1 AND sp.deleted_at IS NULL
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE)
			   AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY sp.created_at DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionSmartOrderRuns:
		return r.readProjectionRows(ctx, actor, `
			SELECT sr.id, jsonb_build_object(
				'run_number', sr.run_number, 'status', sr.status, 'filename', sr.original_filename,
				'total_rows', sr.total_rows, 'matched_rows', sr.matched_rows,
				'unmatched_rows', sr.unmatched_rows, 'estimated_total', sr.estimated_total::text,
				'created_at', sr.created_at)
			  FROM smartorder.runs sr WHERE sr.organization_id = $1
			   AND ($2::text = '' OR TRUE)
			   AND ($3 = '' OR sr.status = $3)
			   AND ($4::timestamptz IS NULL OR sr.created_at >= $4)
			   AND ($5::timestamptz IS NULL OR sr.created_at <= $5)
			 ORDER BY sr.created_at DESC, sr.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionSmartOrderDetails:
		return r.readProjectionRows(ctx, actor, `
			SELECT sr.id, jsonb_build_object(
				'run_number', sr.run_number, 'status', sr.status, 'filename', sr.original_filename,
				'total_rows', sr.total_rows, 'matched_rows', sr.matched_rows,
				'unmatched_rows', sr.unmatched_rows, 'coverage_blocked_rows', sr.coverage_blocked_rows,
				'quota_blocked_rows', sr.quota_blocked_rows, 'estimated_total', sr.estimated_total::text,
				'failure_reason', COALESCE(sr.failure_reason,''),
				'lines', COALESCE((SELECT jsonb_agg(jsonb_build_object(
					'row', sl.row_number, 'name', sl.raw_name, 'outcome', sl.outcome,
					'reason', sl.outcome_reason, 'quantity', sl.effective_qty))
					FROM smartorder.run_lines sl WHERE sl.run_id = sr.id), '[]'::jsonb))
			  FROM smartorder.runs sr WHERE sr.id = $1 AND sr.organization_id = $2`, []any{q.ID, orgID}, 1, 0, false)
	case assistant.ProjectionDecisionMemory:
		return r.readProjectionRows(ctx, actor, `
			SELECT md.id, jsonb_build_object(
				'key', md.decision_key, 'name', md.norm_name, 'product',
				COALESCE(`+nameExpr("p.name")+`,''), 'confidence', md.confidence::text,
				'reason', COALESCE(md.reason,''), 'hits', md.hit_count,
				'last_used_at', md.last_used_at)
			  FROM catalog.match_decisions md LEFT JOIN catalog.products p ON p.id = md.chosen_product_id
			 WHERE (md.organization_id = $1 OR md.scope = 'platform')
			   AND ($2 = '' OR md.norm_name ILIKE '%' || $2 || '%' OR md.decision_key ILIKE '%' || $2 || '%')
			   AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY md.last_used_at DESC NULLS LAST, md.id DESC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, false)
	case assistant.ProjectionBranchQuota:
		quotaArgs := []any{orgID, q.Search, q.BranchID, q.ProductID, q.Limit + 1, q.Offset}
		return r.readProjectionRows(ctx, actor, `
			SELECT v.id, jsonb_build_object(
				'product', `+nameExpr("p.name")+`, 'branch', COALESCE(`+nameExpr("b.name")+`,''),
				'quota_limit', v.quota_limit,
				'used', COALESCE((SELECT SUM(ol.quantity) FROM commerce.order_lines ol
					JOIN commerce.orders qo ON qo.id=ol.order_id
					WHERE ol.product_variant_id=v.id AND ($3::bigint=0 OR qo.branch_id=$3)
					  AND qo.deleted_at IS NULL AND qo.status NOT IN ('cancelled','failed','returned','refunded')),0),
				'remaining', GREATEST(v.quota_limit-COALESCE((SELECT SUM(ol.quantity) FROM commerce.order_lines ol
					JOIN commerce.orders qo ON qo.id=ol.order_id
					WHERE ol.product_variant_id=v.id AND ($3::bigint=0 OR qo.branch_id=$3)
					  AND qo.deleted_at IS NULL AND qo.status NOT IN ('cancelled','failed','returned','refunded')),0),0))
			  FROM catalog.product_variants v JOIN catalog.products p ON p.id = v.product_id
			  LEFT JOIN org.branches b ON b.id = v.branch_id
			 WHERE v.organization_id <> $1 AND v.status = 'active' AND v.quota_limit IS NOT NULL AND v.quota_limit > 0
			   AND ($4::bigint=0 OR v.product_id=$4)
			   AND ($2 = '' OR `+nameExpr("p.name")+` ILIKE '%' || $2 || '%')
			 ORDER BY v.quota_limit ASC, v.id ASC LIMIT $5 OFFSET $6`, quotaArgs, q.Limit, q.Offset, true)
	case assistant.ProjectionSupplierProfile:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object(
				'name', `+nameExpr("o.name")+`, 'organization_number', o.organization_number,
				'type', o.type, 'status', o.status, 'rating', o.rating,
				'phone', COALESCE(o.phone,''), 'email', COALESCE(o.email,''),
				'address', COALESCE(o.address,''), 'branches',
				 (SELECT COUNT(*) FROM org.branches b WHERE b.organization_id=o.id AND b.deleted_at IS NULL))
			  FROM org.organizations o
			 WHERE o.type IN ('vendor','supplier','company','agency') AND o.deleted_at IS NULL
			   AND $1::bigint > 0 AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			   AND ($2 = '' OR `+nameExpr("o.name")+` ILIKE '%' || $2 || '%' OR o.organization_number ILIKE '%' || $2 || '%')
			 ORDER BY o.rating DESC NULLS LAST, o.id ASC LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionFavourites:
		return r.readProjectionRows(ctx, actor, `
			SELECT uf.product_id, jsonb_build_object(
				'product', `+nameExpr("p.name")+`, 'sku', COALESCE(p.sku,''), 'saved_at', uf.created_at)
			  FROM identity.user_favorites uf JOIN catalog.products p ON p.id = uf.product_id
			 WHERE uf.user_id = $8 AND p.deleted_at IS NULL
			   AND $1::bigint > 0 AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY uf.created_at DESC LIMIT $6 OFFSET $7`, append(args, actor.UserID), q.Limit, q.Offset, false)
	case assistant.ProjectionNotifications:
		return r.readProjectionRows(ctx, actor, `
			SELECT n.id, jsonb_build_object(
				'title', n.title, 'body', n.body, 'status', n.status,
				'is_read', n.is_read, 'created_at', n.created_at)
			  FROM notifications.logs n
			 WHERE (n.user_id = $8 OR n.organization_id = $1)
			   AND ($2::text = '' OR TRUE) AND ($3::text = '' OR TRUE) AND ($4::timestamptz IS NULL OR TRUE) AND ($5::timestamptz IS NULL OR TRUE)
			 ORDER BY n.created_at DESC LIMIT $6 OFFSET $7`, append(args, actor.UserID), q.Limit, q.Offset, false)
	default:
		return assistant.Page[assistant.ProjectionRow]{}, fmt.Errorf("assistant: unsupported pharmacy projection %q", q.Kind)
	}
}
