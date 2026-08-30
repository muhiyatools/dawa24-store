package ui

import (
	"net/http"
	"strings"

	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"

	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminAnalyticsPage renders the visitor analytics dashboard.
func (h *UIHandler) AdminAnalyticsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	analytics := &platformadmin.VisitorAnalytics{
		ByDevice:  map[string]int{},
		ByOS:      map[string]int{},
		ByBrowser: map[string]int{},
	}
	if h.adminSvc != nil {
		if a, err := h.adminSvc.VisitorAnalytics(ctx, 20); err == nil && a != nil {
			analytics = a
		}
	}

	h.renderPage(ctx, w, "render admin analytics", pages.AdminAnalytics(lang, dir, analytics))
}

// AdminAuditPage renders the platform audit trail.
func (h *UIHandler) AdminAuditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	action := strings.TrimSpace(r.URL.Query().Get("action"))
	severity := strings.TrimSpace(r.URL.Query().Get("severity"))

	filter := platformadmin.AuditLogFilter{
		Action: action,
		Search: q,
		Limit:  100,
		Offset: 0,
	}

	var entries []*platformadmin.AuditEntry
	var total int
	if h.adminSvc != nil {
		list, tot, err := h.adminSvc.ListAuditLogWithFilter(ctx, filter)
		if err != nil {
			h.log.WarnContext(ctx, "admin audit: list audit log", "error", err)
		} else {
			for _, e := range list {
				localizeAuditEntry(e)
			}
			entries = list
			total = tot
		}
	}

	values := pages.AdminAuditValues{
		Entries:       entries,
		TotalCount:    total,
		SelectedActor: q,
		ActionFilter:  action,
		Severity:      severity,
	}

	h.renderPage(ctx, w, "render admin audit", pages.AdminAuditPage(values, lang, dir))
}

func localizeAuditEntry(e *platformadmin.AuditEntry) {
	if e == nil {
		return
	}
	e.Severity = "عادي (Info)"
	switch e.Action {
	case "org.registered":
		e.Module = "المنشآت"
		e.ActionLabelAr = "تسجيل منشأة جديدة"
		e.Title = "طلب تسجيل منشأة جديدة"
		e.Description = "تم تقديم ملف ترخيص وسجل تجاري لمنشأة دوائية جديدة"
	case "org.approved", "org.status_updated":
		e.Module = "المنشآت"
		e.ActionLabelAr = "تحديث حالة اعتماد المنشأة"
		e.Title = "اعتماد أو ترخيص منشأة"
		e.Description = "تم التحقق من الوثائق والموافقة على حساب المنشأة"
	case "org.rejected":
		e.Module = "المنشآت"
		e.ActionLabelAr = "رفض اعتماد المنشأة"
		e.Title = "رفض اعتماد منشأة"
		e.Description = "تم رفض ملف المنشأة بسبب عدم استيفاء التراخيص"
		e.Severity = "حرج (Critical)"
	case "org.suspended":
		e.Module = "المنشآت"
		e.ActionLabelAr = "إيقاف المنشأة مؤقتاً"
		e.Title = "إيقاف حساب منشأة"
		e.Description = "تم تعليق حساب المنشأة مؤقتاً لمخالفة اللوائح"
		e.Severity = "متوسط (Warning)"
	case "identity.user.registered":
		e.Module = "المستخدمين"
		e.ActionLabelAr = "تسجيل حساب مستخدم جديد"
		e.Title = "إنشاء حساب مستخدم"
		e.Description = "تم تسجيل عضو أو صيدلي جديد في النظام"
	case "identity.user.status_changed":
		e.Module = "المستخدمين"
		e.ActionLabelAr = "تغيير حالة حساب المستخدم"
		e.Title = "تعديل حالة الحساب"
		e.Description = "تحديث حالة التفعيل أو الإيقاف لحساب المستخدم"
		e.Severity = "متوسط (Warning)"
	case "identity.user.role_assigned":
		e.Module = "الأمان والصلاحيات"
		e.ActionLabelAr = "تعيين دور وصلاحية للمستخدم"
		e.Title = "إسناد صلاحية أمنية"
		e.Description = "تعديل رتبة وصلاحيات المستخدم داخل المنصة"
		e.Severity = "متوسط (Warning)"
	case "identity.user.mfa_reset":
		e.Module = "الأمان والصلاحيات"
		e.ActionLabelAr = "إعادة ضبط التحقق الثنائي (MFA)"
		e.Title = "إعادة ضبط أمني (MFA)"
		e.Description = "إعادة ضبط مفاتيح المصادقة الثنائية لحساب المستخدم"
		e.Severity = "حرج (Critical)"
	case "catalog.product.created", "product.created":
		e.Module = "الكتالوج"
		e.ActionLabelAr = "إضافة صنف دوائي جديد"
		e.Title = "إضافة دواء للكتالوج"
		e.Description = "إدراج صنف دوائي ومستحضر معتمد في الكتالوج الموحد"
	case "catalog.product.updated", "product.updated":
		e.Module = "الكتالوج"
		e.ActionLabelAr = "تعديل بيانات الصنف الدوائي"
		e.Title = "تحديث بيانات دواء"
		e.Description = "تعديل الأسعار أو المادة الفعالة أو بيانات الصنف"
	case "catalog.product.deleted", "product.deleted":
		e.Module = "الكتالوج"
		e.ActionLabelAr = "حذف صنف من الكتالوج"
		e.Title = "حذف صنف دوائي"
		e.Description = "إلغاء أو حذف صنف دوائي من الكتالوج المعتمد"
		e.Severity = "حرج (Critical)"
	case "catalog.variant.created", "variant.created":
		e.Module = "عروض الموردين"
		e.ActionLabelAr = "إضافة عرض توريد جديد"
		e.Title = "إضافة عرض سعر دوائي"
		e.Description = "طرح عرض أسعار وتوريد جديد لصنف معتمد"
	case "order.created":
		e.Module = "أوامر التوريد"
		e.ActionLabelAr = "إنشاء طلب توريد جديد"
		e.Title = "إنشاء أمر توريد"
		e.Description = "تم تقديم أمر توريد دوائي جديد من صيدلية"
	case "order.status_updated", "order.status_changed":
		e.Module = "أوامر التوريد"
		e.ActionLabelAr = "تحديث حالة أمر التوريد"
		e.Title = "تحديث حالة الشحن/التوريد"
		e.Description = "تغيير حالة الطلب الدوائي بين التجهيز والتوصيل والاستلام"
	case "institutional_work.created":
		e.Module = "الهيكل المؤسسي"
		e.ActionLabelAr = "إضافة تصنيف هيكل مؤسسي"
		e.Title = "إضافة هيكل مؤسسي جديد"
		e.Description = "إنشاء تصنيف هيكلي جديد للمنشآت والمستودعات"
	case "institutional_work.updated":
		e.Module = "الهيكل المؤسسي"
		e.ActionLabelAr = "تعديل تصنيف هيكل مؤسسي"
		e.Title = "تعديل هيكل مؤسسي"
		e.Description = "تحديث بيانات تصنيف هيكلي أو باقة التسعير"
	default:
		e.Module = "النظام"
		e.ActionLabelAr = e.Action
		e.Title = e.Action
		e.Description = "عملية إدارية مسجلة بالنظام"
	}

	switch e.EntityType {
	case "organization", "org":
		e.EntityTypeAr = "منشأة / شركة"
	case "identity.user", "user":
		e.EntityTypeAr = "مستخدم"
	case "catalog.product", "product":
		e.EntityTypeAr = "صنف دوائي"
	case "catalog.variant", "product_variant", "variant":
		e.EntityTypeAr = "عرض توريد"
	case "order", "commerce.order":
		e.EntityTypeAr = "أمر توريد"
	case "branch", "org.branch":
		e.EntityTypeAr = "فرع مستودع / صيدلية"
	case "institutional_work":
		e.EntityTypeAr = "هيكل مؤسسي"
	default:
		e.EntityTypeAr = e.EntityType
	}

	if e.ActorName == "" {
		e.ActorName = "النظام / System"
	}
	if e.OrganizationName == "" {
		e.OrganizationName = "المنصة الرئيسية"
	}
	if e.IPAddress == "" {
		e.IPAddress = "127.0.0.1 (Local)"
	}
	if e.Route == "" {
		e.Route = "/admin/" + e.EntityType
	}
}
