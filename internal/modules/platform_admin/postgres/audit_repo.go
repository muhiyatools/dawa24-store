package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// ListAuditLogByOrg returns audit trail entries filtered to a specific organization.
func (r *Repository) ListAuditLogByOrg(ctx context.Context, orgID int64, limit, offset int) ([]*platformadmin.AuditEntry, error) {
	list, _, err := r.ListAuditLogByOrgWithTotal(ctx, orgID, limit, offset)
	return list, err
}

// ListAuditLogByOrgWithTotal returns paginated audit trail entries filtered to a specific organization with total count.
func (r *Repository) ListAuditLogByOrgWithTotal(ctx context.Context, orgID int64, limit, offset int) ([]*platformadmin.AuditEntry, int, error) {
	var list []*platformadmin.AuditEntry
	var total int
	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		if err := tx.QueryRow(txCtx, `SELECT count(*) FROM platform.audit_log WHERE organization_id = $1;`, orgID).Scan(&total); err != nil {
			return err
		}

		const query = `
			SELECT a.id, a.organization_id,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.legal_name, ''), 'المنصة الرئيسية') AS org_name,
			       a.actor_user_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email, 'النظام / System') AS actor_name,
			       COALESCE(u.email, '') AS actor_email,
			       a.action, a.entity_type, a.entity_id,
			       COALESCE(HOST(a.ip), '') AS ip_addr,
			       COALESCE(a.request_id, '') AS req_id,
			       a.before, a.after, a.created_at
			FROM platform.audit_log a
			LEFT JOIN identity.users u ON a.actor_user_id = u.id
			LEFT JOIN org.organizations o ON a.organization_id = o.id
			WHERE a.organization_id = $1
			ORDER BY a.created_at DESC, a.id DESC
			LIMIT $2 OFFSET $3;
		`
		if limit <= 0 || limit > 100 {
			limit = 25
		}
		rows, err := tx.Query(txCtx, query, orgID, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e platformadmin.AuditEntry
			var ipAddr, reqID string
			if err := rows.Scan(
				&e.ID, &e.OrganizationID, &e.OrganizationName, &e.ActorUserID,
				&e.ActorName, &e.ActorEmail, &e.Action, &e.EntityType, &e.EntityID,
				&ipAddr, &reqID, &e.Before, &e.After, &e.CreatedAt,
			); err != nil {
				return err
			}
			e.IPAddress = ipAddr
			e.Route = reqID
			enrichAuditEntry(&e)
			list = append(list, &e)
		}
		return rows.Err()
	})
	return list, total, err
}

// ListAuditLogWithFilter returns filtered audit log entries and total count matching the criteria.
func (r *Repository) ListAuditLogWithFilter(ctx context.Context, filter platformadmin.AuditLogFilter) ([]*platformadmin.AuditEntry, int, error) {
	if filter.Limit <= 0 || filter.Limit > 10000 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	var list []*platformadmin.AuditEntry
	var total int

	err := r.db.InReadTx(database.AsSystem(ctx), func(txCtx context.Context, tx pgx.Tx) error {
		var conditions []string
		var args []any
		argIdx := 1

		if filter.OrganizationID != nil && *filter.OrganizationID > 0 {
			conditions = append(conditions, fmt.Sprintf("a.organization_id = $%d", argIdx))
			args = append(args, *filter.OrganizationID)
			argIdx++
		}

		if filter.ActorUserID != nil && *filter.ActorUserID > 0 {
			conditions = append(conditions, fmt.Sprintf("a.actor_user_id = $%d", argIdx))
			args = append(args, *filter.ActorUserID)
			argIdx++
		}

		if strings.TrimSpace(filter.Action) != "" {
			conditions = append(conditions, fmt.Sprintf("(a.action ILIKE '%%' || $%d || '%%' OR a.entity_type ILIKE '%%' || $%d || '%%')", argIdx, argIdx))
			args = append(args, strings.TrimSpace(filter.Action))
			argIdx++
		}

		if strings.TrimSpace(filter.EntityType) != "" {
			conditions = append(conditions, fmt.Sprintf("a.entity_type = $%d", argIdx))
			args = append(args, strings.TrimSpace(filter.EntityType))
			argIdx++
		}

		if filter.DateFrom != nil {
			conditions = append(conditions, fmt.Sprintf("a.created_at >= $%d", argIdx))
			args = append(args, *filter.DateFrom)
			argIdx++
		}

		if filter.DateTo != nil {
			conditions = append(conditions, fmt.Sprintf("a.created_at < $%d", argIdx))
			args = append(args, filter.DateTo.Add(24*time.Hour))
			argIdx++
		}

		if strings.TrimSpace(filter.Search) != "" {
			s := strings.TrimSpace(filter.Search)
			conditions = append(conditions, fmt.Sprintf("(u.name->>'ar' ILIKE '%%' || $%d || '%%' OR u.name->>'en' ILIKE '%%' || $%d || '%%' OR u.email ILIKE '%%' || $%d || '%%' OR o.trade_name->>'ar' ILIKE '%%' || $%d || '%%' OR a.action ILIKE '%%' || $%d || '%%' OR a.entity_id ILIKE '%%' || $%d || '%%')", argIdx, argIdx, argIdx, argIdx, argIdx, argIdx))
			args = append(args, s)
			argIdx++
		}

		whereClause := ""
		if len(conditions) > 0 {
			whereClause = "WHERE " + strings.Join(conditions, " AND ")
		}

		countQuery := fmt.Sprintf(`
			SELECT COUNT(*)
			FROM platform.audit_log a
			LEFT JOIN identity.users u ON a.actor_user_id = u.id
			LEFT JOIN org.organizations o ON a.organization_id = o.id
			%s;
		`, whereClause)

		if err := tx.QueryRow(txCtx, countQuery, args...).Scan(&total); err != nil {
			return err
		}

		query := fmt.Sprintf(`
			SELECT a.id, a.organization_id,
			       COALESCE(NULLIF(o.trade_name->>'ar', ''), NULLIF(o.legal_name, ''), 'المنصة الرئيسية') AS org_name,
			       a.actor_user_id,
			       COALESCE(NULLIF(u.name->>'ar', ''), NULLIF(u.name->>'en', ''), u.email, 'النظام / System') AS actor_name,
			       COALESCE(u.email, '') AS actor_email,
			       a.action, a.entity_type, a.entity_id,
			       COALESCE(HOST(a.ip), '') AS ip_addr,
			       COALESCE(a.request_id, '') AS req_id,
			       a.before, a.after, a.created_at
			FROM platform.audit_log a
			LEFT JOIN identity.users u ON a.actor_user_id = u.id
			LEFT JOIN org.organizations o ON a.organization_id = o.id
			%s
			ORDER BY a.created_at DESC
			LIMIT $%d OFFSET $%d;
		`, whereClause, argIdx, argIdx+1)

		args = append(args, filter.Limit, filter.Offset)

		rows, err := tx.Query(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			var e platformadmin.AuditEntry
			var ipAddr, reqID string
			if err := rows.Scan(
				&e.ID, &e.OrganizationID, &e.OrganizationName, &e.ActorUserID,
				&e.ActorName, &e.ActorEmail, &e.Action, &e.EntityType, &e.EntityID,
				&ipAddr, &reqID, &e.Before, &e.After, &e.CreatedAt,
			); err != nil {
				return err
			}
			e.IPAddress = ipAddr
			e.Route = reqID
			enrichAuditEntry(&e)
			list = append(list, &e)
		}
		return rows.Err()
	})

	return list, total, err
}

// enrichAuditEntry generates human-readable Arabic title, description, module and severity for audit entries.
func enrichAuditEntry(e *platformadmin.AuditEntry) {
	if e == nil {
		return
	}

	actor := e.ActorName
	if actor == "" {
		actor = "مستخدم النظام"
	}

	switch strings.ToLower(e.EntityType) {
	case "organization", "org":
		e.EntityTypeAr = "منشأة"
		e.Module = "إدارة المنشآت"
	case "user", "identity.user":
		e.EntityTypeAr = "مستخدم"
		e.Module = "المستخدمين والهوية"
	case "role", "admin_role":
		e.EntityTypeAr = "دور وصلاحية"
		e.Module = "الأدوار والصلاحيات"
	case "ad", "promo.ad":
		e.EntityTypeAr = "إعلان ترويجي"
		e.Module = "الإعلانات والرعايات"
	case "offer", "promo.offer", "special_offer":
		e.EntityTypeAr = "عرض خاص"
		e.Module = "العروض الترويجية"
	case "offer_package", "promo.offer_package":
		e.EntityTypeAr = "باقة رعاية"
		e.Module = "باقات العروض والرعايات"
	case "sponsorship_request", "adv_product":
		e.EntityTypeAr = "رعاية منتج"
		e.Module = "رعاية وتثبيت المنتجات"
	case "product", "catalog.product":
		e.EntityTypeAr = "منتج بالكتالوج"
		e.Module = "الكتالوج والمخزون"
	case "order", "commerce.order":
		e.EntityTypeAr = "أمر توريد / طلب شراء"
		e.Module = "المبيعات والطلبات"
	case "invoice", "billing.invoice":
		e.EntityTypeAr = "فاتورة مالية"
		e.Module = "المالية والفواتير"
	case "wallet", "billing.wallet":
		e.EntityTypeAr = "محفظة رصيد"
		e.Module = "المحافظ والمدفوعات"
	case "setting", "platform.setting":
		e.EntityTypeAr = "إعدادات المنصة"
		e.Module = "إدارة النظام"
	case "trash", "recycle_bin":
		e.EntityTypeAr = "سلة المهملات"
		e.Module = "صيانة النظام"
	default:
		e.EntityTypeAr = e.EntityType
		if e.EntityTypeAr == "" {
			e.EntityTypeAr = "عنصر بالنظام"
		}
		e.Module = "إدارة المنصة"
	}

	e.Severity = "info"

	// Check if there is an i18n key defined for this action
	if label, ok := i18n.Lookup(i18n.AR, "audit.action."+e.Action); ok && label != "" {
		e.ActionLabelAr = label
	}

	act := strings.ToLower(e.Action)
	switch {
	case strings.Contains(act, "registered") || act == "org.registered":
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "تسجيل منشأة"
		}
		e.Title = "تسجيل منشأة جديدة في المنصة"
		orgName := e.OrganizationName
		if orgName == "" {
			orgName = fmt.Sprintf("منشأة #%s", e.EntityID)
		}
		e.Description = fmt.Sprintf("قام %s بتسجيل منشأة جديدة (%s) برقم تعريفي #%s بانتظار استكمال الوثائق والمراجعة الإدارية.", actor, orgName, e.EntityID)
		e.Severity = "success"

	case strings.Contains(act, "approved") || strings.Contains(act, "approve"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "اعتماد وموافقة"
		}
		e.Title = fmt.Sprintf("اعتماد %s بنجاح", e.EntityTypeAr)
		e.Description = fmt.Sprintf("قام %s بالموافقة واعتماد %s رقم #%s لصالح المنشأة (%s).", actor, e.EntityTypeAr, e.EntityID, e.OrganizationName)
		e.Severity = "success"

	case strings.Contains(act, "rejected") || strings.Contains(act, "reject"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "رفض طلب"
		}
		e.Title = fmt.Sprintf("رفض %s", e.EntityTypeAr)
		e.Description = fmt.Sprintf("قام %s برفض %s رقم #%s الخاص بالمنشأة (%s) مع إرسال الملاحظات للمورد.", actor, e.EntityTypeAr, e.EntityID, e.OrganizationName)
		e.Severity = "warning"

	case strings.Contains(act, "suspend"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "إيقاف مؤقت"
		}
		e.Title = fmt.Sprintf("إيقاف %s", e.EntityTypeAr)
		e.Description = fmt.Sprintf("قام %s بتعليق وإيقاف %s رقم #%s في النظام.", actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "warning"

	case strings.Contains(act, "reactivate"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "إعادة تفعيل"
		}
		e.Title = fmt.Sprintf("إعادة تفعيل %s", e.EntityTypeAr)
		e.Description = fmt.Sprintf("قام %s بإعادة تفعيل %s رقم #%s في النظام.", actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "success"

	case strings.Contains(act, "toggled") || strings.Contains(act, "status_changed") || strings.Contains(act, "toggle"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "تغيير الحالة"
		}
		e.Title = fmt.Sprintf("تحديث حالة تفعيل %s", e.EntityTypeAr)
		e.Description = fmt.Sprintf("قام %s بتعديل حالة تفعيل %s رقم #%s على المنصة.", actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "info"

	case strings.Contains(act, "trash.purge") || strings.Contains(act, "purge"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "حذف نهائي"
		}
		e.Title = "تفريغ وحذف نهائي من سلة المهملات"
		e.Description = fmt.Sprintf("قام %s بتنفيذ عملية تفريغ وحذف نهائي للعناصر من سلة المهملات (%s) لمعرف #%s.", actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "critical"

	case strings.Contains(act, "trash.restore") || strings.Contains(act, "restore"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "استعادة محذوف"
		}
		e.Title = "استعادة عنصر من سلة المهملات"
		e.Description = fmt.Sprintf("قام %s باستعادة %s رقم #%s من سلة المهملات وإعادته للحالة النشطة.", actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "info"

	case strings.Contains(act, "create") || strings.Contains(act, "insert"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "إنشاء جديد"
		}
		e.Title = fmt.Sprintf("إنشاء %s جديد", e.EntityTypeAr)
		e.Description = fmt.Sprintf("قام %s بإنشاء %s جديد برقم تعريفي #%s بنجاح.", actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "success"

	case strings.Contains(act, "update") || strings.Contains(act, "edit") || strings.Contains(act, "modify"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "تعديل بيانات"
		}
		e.Title = fmt.Sprintf("تعديل بيانات %s", e.EntityTypeAr)
		e.Description = fmt.Sprintf("قام %s بتحديث وتعديل بيانات %s رقم #%s في النظام.", actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "info"

	case strings.Contains(act, "delete") || strings.Contains(act, "remove"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "حذف سجل"
		}
		e.Title = fmt.Sprintf("حذف %s من النظام", e.EntityTypeAr)
		e.Description = fmt.Sprintf("قام %s بحذف %s رقم #%s ونقله للمهملات أو إزالته.", actor, e.EntityTypeAr, e.EntityID)
		e.Severity = "warning"

	case strings.Contains(act, "login") || strings.Contains(act, "auth.sign_in"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "تسجيل دخول"
		}
		e.Title = "تسجيل دخول ناجح إلى النظام"
		e.Description = fmt.Sprintf("قام المستخدم %s بتسجيل الدخول بنجاح من العنوان %s.", actor, e.IPAddress)
		e.Severity = "info"

	case strings.Contains(act, "logout") || strings.Contains(act, "auth.sign_out"):
		if e.ActionLabelAr == "" {
			e.ActionLabelAr = "تسجيل خروج"
		}
		e.Title = "تسجيل خروج من النظام"
		e.Description = fmt.Sprintf("قام المستخدم %s بتسجيل الخروج من الجلسة النشطة.", actor)
		e.Severity = "info"

	default:
		if e.ActionLabelAr == "" {
			parts := strings.Split(e.Action, ".")
			actionName := e.Action
			if len(parts) > 1 {
				actionName = parts[len(parts)-1]
			}
			e.ActionLabelAr = actionName
		}
		e.Title = fmt.Sprintf("إجراء: %s على %s", e.Action, e.EntityTypeAr)
		e.Description = fmt.Sprintf("قام %s بتنفيذ عملية (%s) على %s برقم #%s.", actor, e.Action, e.EntityTypeAr, e.EntityID)
	}
}
