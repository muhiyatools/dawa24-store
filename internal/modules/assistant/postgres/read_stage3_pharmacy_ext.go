package postgres

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (r *Repository) readPharmacyProjectionExt(
	ctx context.Context, actor authctx.Actor, q assistant.ProjectionQuery,
) (assistant.Page[assistant.ProjectionRow], error) {
	orgID, err := scopeOf(actor)
	if err != nil {
		return assistant.Page[assistant.ProjectionRow]{}, err
	}
	args := projectionArgs(orgID, q)
	switch q.Kind {
	case assistant.ProjectionBranchProductAvailability:
		branchArgs := []any{orgID, q.Search, q.BranchID, q.Limit + 1, q.Offset}
		return r.readProjectionRows(ctx, actor, `
			SELECT v.product_id, jsonb_build_object(
				'product', `+nameExpr("p.name")+`,
				'sku', COALESCE(v.sku, ''),
				'supplier', COALESCE(`+nameExpr("org.name")+`, ''),
				'price', v.price::text,
				'discount', v.discount::text,
				'unit', COALESCE(v.unit, ''),
				'stock', COALESCE((SELECT SUM(s.quantity) FROM inventory.stocks s WHERE s.product_variant_id = v.id AND s.deleted_at IS NULL), 0),
				'quota_remaining', CASE WHEN v.quota_limit IS NOT NULL AND v.quota_limit > 0 THEN
					GREATEST(v.quota_limit - COALESCE((SELECT SUM(ol.quantity) FROM commerce.order_lines ol JOIN commerce.orders qo ON qo.id=ol.order_id WHERE ol.product_variant_id=v.id AND ($3::bigint=0 OR qo.branch_id=$3) AND qo.deleted_at IS NULL AND qo.status NOT IN ('cancelled','failed','returned','refunded')),0), 0)
					ELSE NULL END,
				'status', v.status)
			  FROM catalog.product_variants v
			  JOIN catalog.products p ON p.id = v.product_id
			  LEFT JOIN org.organizations org ON org.id = v.organization_id
			 WHERE v.organization_id <> $1 AND v.status = 'active' AND v.deleted_at IS NULL
			   AND p.deleted_at IS NULL AND p.status = 'active'
			   AND ($2 = '' OR `+nameExpr("p.name")+` ILIKE '%' || $2 || '%' OR COALESCE(v.sku,'') ILIKE '%' || $2 || '%' OR COALESCE(p.scientific_name,'') ILIKE '%' || $2 || '%')
			 ORDER BY (v.discount > 0) DESC, v.price ASC, v.id ASC
			 LIMIT $4 OFFSET $5`, branchArgs, q.Limit, q.Offset, true)
	case assistant.ProjectionOrderWorkflowRules:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object(
				'supplier_name', `+nameExpr("o.name")+`,
				'type', o.type,
				'rating', o.rating,
				'phone', COALESCE(o.phone,''),
				'min_order_amount', COALESCE((SELECT MIN(op.min_order_amount) FROM promo.offers op WHERE op.organization_id = o.id AND op.is_active = true AND op.deleted_at IS NULL), 0)::text,
				'delivery_days', COALESCE((SELECT jsonb_agg(DISTINCT wc.day_of_week) FROM workflow.weekly_coverages wc WHERE wc.organization_id = o.id AND wc.is_active = true), '[]'::jsonb),
				'delivery_windows', COALESCE((SELECT jsonb_agg(jsonb_build_object('day', wc.day_of_week, 'from', wc.coverage_from, 'to', wc.coverage_to)) FROM workflow.weekly_coverages wc WHERE wc.organization_id = o.id AND wc.is_active = true LIMIT 5), '[]'::jsonb))
			  FROM org.organizations o
			 WHERE o.type IN ('vendor','supplier','company') AND o.deleted_at IS NULL
			   AND ($2 = '' OR `+nameExpr("o.name")+` ILIKE '%' || $2 || '%')
			 ORDER BY o.rating DESC NULLS LAST, o.id ASC
			 LIMIT $6 OFFSET $7`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionFinancialObligations:
		return r.readProjectionRows(ctx, actor, `
			SELECT 1::bigint, jsonb_build_object(
				'unpaid_invoices_total', COALESCE((SELECT SUM(i.total_amount) FROM billing.invoices i WHERE (i.customer_org_id = $1 OR i.organization_id = $1) AND i.payment_status IN ('unpaid','partially_paid','pending')), 0)::text,
				'unpaid_invoices_count', (SELECT COUNT(*) FROM billing.invoices i WHERE (i.customer_org_id = $1 OR i.organization_id = $1) AND i.payment_status IN ('unpaid','partially_paid','pending')),
				'in_flight_orders_total', COALESCE((SELECT SUM(o.total_amount) FROM commerce.orders o WHERE o.organization_id = $1 AND o.status IN ('confirmed','processing','shipped') AND o.deleted_at IS NULL), 0)::text,
				'in_flight_orders_count', (SELECT COUNT(*) FROM commerce.orders o WHERE o.organization_id = $1 AND o.status IN ('confirmed','processing','shipped') AND o.deleted_at IS NULL),
				'wallet_balance', COALESCE((SELECT wt.balance_after FROM billing.wallet_transactions wt JOIN billing.wallets w ON w.id = wt.wallet_id WHERE w.organization_id = $1 ORDER BY wt.id DESC LIMIT 1), 0)::text,
				'currency', 'EGP')`, []any{orgID}, 1, 0, false)
	default:
		return assistant.Page[assistant.ProjectionRow]{}, fmt.Errorf("assistant: unsupported pharmacy projection %q", q.Kind)
	}
}
