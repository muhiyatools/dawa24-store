package postgres

import (
	"context"
	"fmt"

	"github.com/muhiya/dawa24-store/internal/modules/assistant"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
)

func (r *Repository) readAdminProjection(
	ctx context.Context, actor authctx.Actor, q assistant.ProjectionQuery,
) (assistant.Page[assistant.ProjectionRow], error) {
	if err := requireStaff(actor); err != nil {
		return assistant.Page[assistant.ProjectionRow]{}, err
	}
	args := adminProjectionArgs(q)
	switch q.Kind {
	case assistant.ProjectionOrganizations:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object('name', `+nameExpr("o.name")+`,
				'organization_number', o.organization_number, 'type', o.type, 'status', o.status,
				'city', COALESCE(o.address,''), 'created_at', o.created_at)
			  FROM org.organizations o WHERE o.deleted_at IS NULL
			   AND ($1='' OR `+nameExpr("o.name")+` ILIKE '%' || $1 || '%' OR o.organization_number ILIKE '%' || $1 || '%')
			   AND ($2='' OR o.status=$2)
			   AND ($3::timestamptz IS NULL OR TRUE) AND ($4::timestamptz IS NULL OR TRUE)
			 ORDER BY o.created_at DESC, o.id DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionOrganizationDetails:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object('name', `+nameExpr("o.name")+`,
				'organization_number', o.organization_number, 'type', o.type, 'status', o.status,
				'email', COALESCE(o.email,''), 'phone', COALESCE(o.phone,''), 'address', COALESCE(o.address,''),
				'rating', o.rating, 'branches', COALESCE((SELECT jsonb_agg(jsonb_build_object(
					'name', `+nameExpr("b.name")+`, 'status', b.status, 'main', b.is_main))
					FROM org.branches b WHERE b.organization_id=o.id AND b.deleted_at IS NULL),'[]'::jsonb),
				'warehouses', COALESCE((SELECT jsonb_agg(jsonb_build_object('name',w.name,'code',w.code,'active',w.is_active))
					FROM inventory.warehouses w WHERE w.organization_id=o.id AND w.deleted_at IS NULL),'[]'::jsonb))
			  FROM org.organizations o WHERE o.id=$1 AND o.deleted_at IS NULL`, []any{q.ID}, 1, 0, true)
	case assistant.ProjectionApprovals:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object('name', `+nameExpr("o.name")+`,
				'type', o.type, 'status', o.status, 'created_at', o.created_at,
				'documents', (SELECT COUNT(*) FROM platform_admin.documents d WHERE d.organization_id=o.id AND d.status='pending'))
			  FROM org.organizations o WHERE o.status='pending' AND o.deleted_at IS NULL
			   AND ($1='' OR `+nameExpr("o.name")+` ILIKE '%' || $1 || '%')
			   AND ($2::text = '' OR TRUE) AND ($3::timestamptz IS NULL OR TRUE) AND ($4::timestamptz IS NULL OR TRUE)
			 ORDER BY o.created_at ASC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionDeletionRequests:
		return r.readProjectionRows(ctx, actor, `
			SELECT dr.id, jsonb_build_object('organization', COALESCE(`+nameExpr("o.name")+`,''),
				'requester', COALESCE(`+nameExpr("u.name")+`,u.email,''), 'status', dr.status,
				'reason', COALESCE(dr.reason,''), 'created_at', dr.created_at)
			  FROM org.organization_deletion_requests dr
			  LEFT JOIN org.organizations o ON o.id=dr.organization_id
			  LEFT JOIN identity.users u ON u.id=dr.requested_by
			 WHERE ($2='' OR dr.status=$2)
			   AND ($1::text = '' OR TRUE) AND ($3::timestamptz IS NULL OR TRUE) AND ($4::timestamptz IS NULL OR TRUE)
			 ORDER BY dr.created_at DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionUsers:
		return r.readProjectionRows(ctx, actor, `
			SELECT u.id, jsonb_build_object('member', COALESCE(`+nameExpr("u.name")+`,u.email,''),
				'email', u.email, 'phone', COALESCE(u.phone,''), 'status', u.status,
				'role', COALESCE(u.role,''), 'created_at', u.created_at, 'last_login_at', us.last_login_at)
			  FROM identity.users u LEFT JOIN identity.user_security us ON us.user_id=u.id
			 WHERE u.deleted_at IS NULL AND ($1='' OR `+nameExpr("u.name")+` ILIKE '%' || $1 || '%' OR u.email ILIKE '%' || $1 || '%' OR u.phone ILIKE '%' || $1 || '%')
			   AND ($2='' OR u.status=$2) AND ($3::timestamptz IS NULL OR TRUE) AND ($4::timestamptz IS NULL OR TRUE)
			 ORDER BY u.created_at DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionUserDetails:
		return r.readProjectionRows(ctx, actor, `
			SELECT u.id, jsonb_build_object('member', COALESCE(`+nameExpr("u.name")+`,u.email,''),
				'email', u.email, 'phone', COALESCE(u.phone,''), 'status', u.status, 'role', COALESCE(u.role,''),
				'created_at', u.created_at, 'last_login_at', us.last_login_at,
				'organisations', COALESCE((SELECT jsonb_agg(jsonb_build_object(
					'organization', COALESCE(`+nameExpr("org2.name")+`,''), 'status', uo.status))
					FROM org.user_organizations uo
					LEFT JOIN org.organizations org2 ON org2.id = COALESCE(uo.customer_org_id, uo.vendor_org_id)
					WHERE uo.user_id=u.id),'[]'::jsonb))
			  FROM identity.users u LEFT JOIN identity.user_security us ON us.user_id=u.id
			 WHERE u.id=$1 AND u.deleted_at IS NULL`, []any{q.ID}, 1, 0, true)
	case assistant.ProjectionErrorLogs:
		return r.readProjectionRows(ctx, actor, `
			SELECT e.id, jsonb_build_object('level', e.error_level, 'message', e.error_message,
				'exception', COALESCE(e.exception_class,''), 'path', COALESCE(e.url_path,''),
				'http_status', e.status, 'created_at', e.created_at)
			  FROM platform_admin.error_logs e
			 WHERE ($1='' OR e.error_message ILIKE '%' || $1 || '%' OR e.url_path ILIKE '%' || $1 || '%' OR e.exception_class ILIKE '%' || $1 || '%')
			   AND ($2::text = '' OR TRUE)
			   AND ($3::timestamptz IS NULL OR e.created_at >= $3) AND ($4::timestamptz IS NULL OR e.created_at <= $4)
			 ORDER BY e.created_at DESC, e.id DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionAuditLog:
		return r.readProjectionRows(ctx, actor, `
			SELECT a.id, jsonb_build_object('action', a.action, 'entity_type', a.entity_type,
				'entity', a.entity_id, 'actor', COALESCE(`+nameExpr("u.name")+`,u.email,''),
				'organization', COALESCE(`+nameExpr("o.name")+`,''), 'created_at', a.created_at)
			  FROM platform.audit_log a LEFT JOIN identity.users u ON u.id=a.actor_user_id
			  LEFT JOIN org.organizations o ON o.id=a.organization_id
			 WHERE ($1='' OR a.action ILIKE '%' || $1 || '%' OR a.entity_type ILIKE '%' || $1 || '%' OR a.entity_id ILIKE '%' || $1 || '%')
			   AND ($2::text = '' OR TRUE)
			   AND ($3::timestamptz IS NULL OR a.created_at >= $3) AND ($4::timestamptz IS NULL OR a.created_at <= $4)
			 ORDER BY a.created_at DESC, a.id DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionFinance:
		return r.readProjectionRows(ctx, actor, `
			SELECT 1::bigint, jsonb_build_object(
				'wallet_balance', COALESCE((SELECT SUM(wt.balance_after) FROM billing.wallet_transactions wt),0)::text,
				'invoices', (SELECT COUNT(*) FROM billing.invoices i WHERE ($3::timestamptz IS NULL OR i.created_at >= $3) AND ($4::timestamptz IS NULL OR i.created_at <= $4)),
				'payments', COALESCE((SELECT SUM(p.amount) FROM billing.payments p WHERE p.status IN ('paid','completed') AND ($3::timestamptz IS NULL OR p.created_at >= $3) AND ($4::timestamptz IS NULL OR p.created_at <= $4)),0)::text,
				'pending_withdrawals', (SELECT COUNT(*) FROM billing.wallet_withdrawals w WHERE w.status IN ('pending','requested')),
				'orders', (SELECT COUNT(*) FROM commerce.orders o WHERE ($3::timestamptz IS NULL OR o.created_at >= $3) AND ($4::timestamptz IS NULL OR o.created_at <= $4)))
			 WHERE ($1::text = '' OR TRUE) AND ($2::text = '' OR TRUE)`, args[:4], 1, 0, true)
	case assistant.ProjectionWalletTransactions:
		return r.readProjectionRows(ctx, actor, `
			SELECT wt.id, jsonb_build_object('organization', `+nameExpr("o.name")+`,
				'type', wt.type, 'amount', wt.amount::text, 'balance_after', wt.balance_after::text,
				'description', COALESCE(wt.description,''), 'created_at', wt.created_at)
			  FROM billing.wallet_transactions wt JOIN billing.wallets w ON w.id=wt.wallet_id
			  LEFT JOIN org.organizations o ON o.id=w.organization_id
			 WHERE ($1='' OR `+nameExpr("o.name")+` ILIKE '%' || $1 || '%' OR wt.type ILIKE '%' || $1 || '%')
			   AND ($2::text = '' OR TRUE)
			   AND ($3::timestamptz IS NULL OR wt.created_at >= $3) AND ($4::timestamptz IS NULL OR wt.created_at <= $4)
			 ORDER BY wt.created_at DESC, wt.id DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionSubscriptions:
		return r.readProjectionRows(ctx, actor, `
			SELECT s.id, jsonb_build_object('organisation', `+nameExpr("o.name")+`,
				'plan', `+nameExpr("p.name")+`, 'status', s.status, 'starts_at', s.starts_at,
				'expires_at', s.expires_at, 'billing_cycle', COALESCE(s.billing_cycle,''))
			  FROM billing.subscriptions s JOIN billing.plans p ON p.id=s.plan_id
			  LEFT JOIN org.organizations o ON o.id=s.organization_id
			 WHERE ($2='' OR s.status=$2)
			   AND ($1::text = '' OR TRUE) AND ($3::timestamptz IS NULL OR TRUE) AND ($4::timestamptz IS NULL OR TRUE)
			 ORDER BY s.expires_at ASC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionVisitors:
		return r.readProjectionRows(ctx, actor, `
			 SELECT ROW_NUMBER() OVER (ORDER BY v.city)::bigint, jsonb_build_object(
				'city', COALESCE(v.city,''), 'visits', COUNT(*), 'unique_visitors', COUNT(DISTINCT v.visitor_key))
			  FROM platform_admin.visitors v WHERE ($1::text = '' OR TRUE) AND ($2::text = '' OR TRUE)
			   AND ($3::timestamptz IS NULL OR v.visited_at >= $3)
			   AND ($4::timestamptz IS NULL OR v.visited_at <= $4)
			 GROUP BY v.city ORDER BY COUNT(*) DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionHealth:
		return r.readProjectionRows(ctx, actor, `
			SELECT 1::bigint, jsonb_build_object(
				'errors_last_day', (SELECT COUNT(*) FROM platform_admin.error_logs WHERE created_at >= now()-interval '1 day'),
				'queued_jobs', (SELECT COUNT(*) FROM river_queue WHERE created_at >= now()-interval '1 day'),
				'failed_jobs', (SELECT COUNT(*) FROM river_queue WHERE metadata::text ILIKE '%failed%'),
				'checked_at', now())`, nil, 1, 0, true)
	case assistant.ProjectionMatchDecisions:
		return r.readProjectionRows(ctx, actor, `
			 SELECT md.id, jsonb_build_object('key', md.decision_key, 'name', md.norm_name,
				'confidence', md.confidence::text, 'scope', md.scope, 'hits', md.hit_count,
				'created_at', md.created_at, 'last_used_at', md.last_used_at)
			  FROM catalog.match_decisions md WHERE ($1='' OR md.decision_key ILIKE '%' || $1 || '%' OR md.norm_name ILIKE '%' || $1 || '%')
			   AND ($2::text = '' OR TRUE)
			   AND ($3::timestamptz IS NULL OR md.created_at >= $3) AND ($4::timestamptz IS NULL OR md.created_at <= $4)
			 ORDER BY md.created_at DESC, md.id DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionInstitutionalGraph:
		return r.readProjectionRows(ctx, actor, `
			SELECT c.id, jsonb_build_object('from_work', `+nameExpr("fw.title")+`,
				'to_work', `+nameExpr("tw.title")+`, 'created_at', c.created_at,
				'buyers', COALESCE((SELECT COUNT(*) FROM org.branch_institutional_works biw WHERE biw.institutional_work_id=c.from_institutional_work_id),0),
				'suppliers', COALESCE((SELECT COUNT(*) FROM org.branch_institutional_works biw WHERE biw.institutional_work_id=c.to_institutional_work_id),0))
			  FROM org.institutional_work_connections c
			  JOIN org.institutional_works fw ON fw.id=c.from_institutional_work_id
			  JOIN org.institutional_works tw ON tw.id=c.to_institutional_work_id
			 WHERE ($1='' OR `+nameExpr("fw.title")+` ILIKE '%' || $1 || '%' OR `+nameExpr("tw.title")+` ILIKE '%' || $1 || '%')
			   AND ($2::text = '' OR TRUE) AND ($3::timestamptz IS NULL OR TRUE) AND ($4::timestamptz IS NULL OR TRUE)
			 ORDER BY c.created_at DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionAdminOrders:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object(
				'order_number', o.order_number,
				'buyer', COALESCE(`+nameExpr("buy.name")+`,''),
				'branch', COALESCE(`+nameExpr("b.name")+`,''),
				'total_amount', o.total_amount::text,
				'subtotal', o.subtotal_amount::text,
				'discount', o.discount_amount::text,
				'status', o.status,
				'payment_status', o.payment_status,
				'line_count', (SELECT COUNT(*) FROM commerce.order_lines ol WHERE ol.order_id = o.id),
				'placed_at', o.created_at)
			  FROM commerce.orders o
			  LEFT JOIN org.organizations buy ON buy.id = o.organization_id
			  LEFT JOIN org.branches b ON b.id = o.branch_id
			 WHERE o.deleted_at IS NULL
			   AND ($1 = '' OR o.order_number ILIKE '%' || $1 || '%' OR `+nameExpr("buy.name")+` ILIKE '%' || $1 || '%')
			   AND ($2 = '' OR o.status = $2)
			   AND ($3::timestamptz IS NULL OR o.created_at >= $3)
			   AND ($4::timestamptz IS NULL OR o.created_at <= $4)
			 ORDER BY o.created_at DESC, o.id DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionAdminOrderDetails:
		return r.readProjectionRows(ctx, actor, `
			SELECT o.id, jsonb_build_object(
				'order_number', o.order_number,
				'buyer', COALESCE(`+nameExpr("buy.name")+`,''),
				'buyer_phone', COALESCE(buy.phone,''),
				'branch', COALESCE(`+nameExpr("b.name")+`,''),
				'branch_address', COALESCE(b.address,''),
				'total_amount', o.total_amount::text,
				'subtotal', o.subtotal_amount::text,
				'discount', o.discount_amount::text,
				'shipping_fee', o.shipping_fee::text,
				'status', o.status,
				'payment_status', o.payment_status,
				'placed_at', o.created_at,
				'shipments', COALESCE((
					SELECT jsonb_agg(jsonb_build_object(
						'shipment_number', sh.shipment_number,
						'supplier', COALESCE(`+nameExpr("supp.name")+`,''),
						'status', sh.status,
						'total', sh.total_amount::text))
					FROM commerce.order_shipments sh
					LEFT JOIN org.organizations supp ON supp.id = sh.organization_id
					WHERE sh.order_id = o.id), '[]'::jsonb),
				'lines', COALESCE((
					SELECT jsonb_agg(jsonb_build_object(
						'product', ol.product_name,
						'quantity', ol.quantity,
						'unit_price', ol.unit_price::text,
						'total', ol.total_price::text))
					FROM commerce.order_lines ol
					WHERE ol.order_id = o.id), '[]'::jsonb))
			  FROM commerce.orders o
			  LEFT JOIN org.organizations buy ON buy.id = o.organization_id
			  LEFT JOIN org.branches b ON b.id = o.branch_id
			 WHERE o.id = $1`, []any{q.ID}, 1, 0, true)
	case assistant.ProjectionAdminCatalog:
		return r.readProjectionRows(ctx, actor, `
			SELECT p.id, jsonb_build_object(
				'product_name', `+nameExpr("p.name")+`,
				'sku', COALESCE(p.sku,''),
				'barcode', COALESCE(p.barcode,''),
				'scientific_name', COALESCE(p.scientific_name,''),
				'price', p.price::text,
				'discount', p.discount::text,
				'unit', COALESCE(p.unit,''),
				'status', p.status,
				'vendor_count', (SELECT COUNT(DISTINCT v.organization_id) FROM catalog.product_variants v WHERE v.product_id = p.id AND v.deleted_at IS NULL),
				'created_at', p.created_at)
			  FROM catalog.products p
			 WHERE p.deleted_at IS NULL
			   AND ($1 = '' OR `+nameExpr("p.name")+` ILIKE '%' || $1 || '%' OR COALESCE(p.sku,'') ILIKE '%' || $1 || '%' OR COALESCE(p.barcode,'') ILIKE '%' || $1 || '%' OR COALESCE(p.scientific_name,'') ILIKE '%' || $1 || '%')
			   AND ($2 = '' OR p.status = $2)
			   AND ($3::timestamptz IS NULL OR p.created_at >= $3)
			   AND ($4::timestamptz IS NULL OR p.created_at <= $4)
			 ORDER BY p.id DESC LIMIT $5 OFFSET $6`, args, q.Limit, q.Offset, true)
	case assistant.ProjectionSecurityEvents:
		return r.readProjectionRows(ctx, actor, `
			SELECT 1::bigint, jsonb_build_object(
				'failed_logins_24h', (SELECT COUNT(*) FROM platform.audit_log WHERE action = 'user.login.failed' AND created_at >= now() - interval '24 hours'),
				'denied_tool_calls_24h', (SELECT COUNT(*) FROM assistant.tool_audit WHERE decision <> 'allowed' AND created_at >= now() - interval '24 hours'),
				'active_sessions_count', (SELECT COUNT(*) FROM identity.user_security WHERE last_login_at >= now() - interval '24 hours'),
				'recent_audit_alerts', COALESCE((
					SELECT jsonb_agg(jsonb_build_object(
						'action', a.action,
						'actor', COALESCE(`+nameExpr("u.name")+`, u.email, 'unknown'),
						'entity_type', a.entity_type,
						'at', a.created_at))
					FROM platform.audit_log a
					LEFT JOIN identity.users u ON u.id = a.actor_user_id
					WHERE a.created_at >= now() - interval '24 hours'
					ORDER BY a.id DESC LIMIT 5), '[]'::jsonb))`, nil, 1, 0, true)
	default:
		return assistant.Page[assistant.ProjectionRow]{}, fmt.Errorf("assistant: unsupported admin projection %q", q.Kind)
	}
}
