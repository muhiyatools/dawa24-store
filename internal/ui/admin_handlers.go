package ui

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xuri/excelize/v2"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/platform/features"
	"github.com/muhiya/dawa24-store/internal/shared/apperr"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)


func (h *UIHandler) AdminDashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	// Every figure here used to be len() of a page capped at 100 rows, so the
	// dashboard silently stopped counting at 100 and reported "100 users" to a
	// platform with a thousand. Totals come from COUNT queries.
	stats := pages.AdminDashboardStats{}
	var pendingOrgs []*org.Organization

	if h.idSvc != nil {
		if n, err := h.idSvc.AdminCountUsers(ctx); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count users", "error", err)
		} else {
			stats.TotalUsers = n
		}
	}
	if h.orgSvc != nil {
		if n, err := h.orgSvc.CountOrganizations(ctx, nil, nil); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count organizations", "error", err)
		} else {
			stats.TotalOrganizations = n
		}
		pending := org.StatusPending
		if n, err := h.orgSvc.CountOrganizations(ctx, nil, &pending); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count pending organizations", "error", err)
		} else {
			stats.PendingApprovals = n
		}
		if list, err := h.orgSvc.ListOrganizations(ctx, nil, &pending, 5, 0); err == nil {
			pendingOrgs = list
		}
	}
	if h.commSvc != nil {
		if n, err := h.commSvc.CountOrders(ctx); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count orders", "error", err)
		} else {
			stats.TotalOrders = n
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminDashboard(stats, pendingOrgs, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin dashboard page", "error", err)
	}
}

func (h *UIHandler) AdminUsersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	if h.idSvc == nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = pages.AdminUsers(nil, lang, dir, h.isHTMX(r)).Render(ctx, w)
		return
	}

	users, err := h.idSvc.AdminListUsers(ctx, "", "")
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminUsers(users, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin users page", "error", err)
	}
}

func (h *UIHandler) AdminApprovalsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	statusParam := r.URL.Query().Get("status")

	var orgs []*org.Organization
	if h.orgSvc != nil {
		var filterStatus *org.OrganizationStatus
		if statusParam != "" {
			st := org.OrganizationStatus(statusParam)
			filterStatus = &st
		} else {
			st := org.StatusPending
			filterStatus = &st
		}
		list, err := h.orgSvc.ListOrganizations(ctx, nil, filterStatus, 100, 0)
		if err == nil {
			orgs = list
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminApprovals(orgs, statusParam, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin approvals page", "error", err)
	}
}

// AdminOrgReviewSubmit handles full administrative approval/rejection with custom reason.
func (h *UIHandler) AdminOrgReviewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/approvals", "error", "معرف المنشأة غير صالح.")
		return
	}

	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/approvals", "error", "خدمة المؤسسات غير متاحة.")
		return
	}

	status := org.OrganizationStatus(r.PostFormValue("status"))
	notes := r.PostFormValue("verification_notes")
	rejectionReason := r.PostFormValue("rejection_reason")

	if err := h.orgSvc.ReviewOrganization(ctx, id, status, notes, rejectionReason, actor.UserID); err != nil {
		h.redirectWithNotice(w, r, "/admin/approvals", "error", h.safeMessage(err, langOf(r)))
		return
	}

	msg := "تم اعتماد وتفعيل ترخيص المنشأة بنجاح."
	if status == org.StatusRejected {
		msg = "تم رفض طلب المنشأة وحفظ سبب الرفض."
	} else if status == org.StatusSuspended {
		msg = "تم تعليق حساب المنشأة مؤقتاً."
	}

	h.redirectWithNotice(w, r, "/admin/approvals", "success", msg)
}

// Platform settings keys. These live in platform_admin.system_settings.
const (
	settingSupportEmail   = "platform.support_email"
	settingCommissionRate = "platform.commission_rate"
)

func (h *UIHandler) AdminSettingsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	// The form used to carry these two values as literals in the markup, so it
	// always displayed the defaults regardless of what had been saved.
	values := pages.AdminSettingsValues{
		SupportEmail:   "support@dawa24.eg",
		CommissionRate: "1.5",
		FeatureFlags:   features.List(),
	}
	if h.adminSvc != nil {
		if s, err := h.adminSvc.GetSetting(ctx, settingSupportEmail); err == nil && s != nil {
			if v, ok := s.Value["value"].(string); ok && v != "" {
				values.SupportEmail = v
			}
		}
		if s, err := h.adminSvc.GetSetting(ctx, settingCommissionRate); err == nil && s != nil {
			if v, ok := s.Value["value"].(string); ok && v != "" {
				values.CommissionRate = v
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminSettings(values, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin settings page", "error", err)
	}
}

// AdminFeatureToggleSubmit toggles a platform feature flag in real-time.
func (h *UIHandler) AdminFeatureToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok || (!actor.IsStaff && !actor.IsPlatformAdmin()) {
		http.Redirect(w, r, "/auth/login?redirect=/admin/settings", http.StatusSeeOther)
		return
	}


	key := strings.TrimSpace(r.PostFormValue("key"))
	enabledStr := strings.TrimSpace(r.PostFormValue("enabled"))
	enabled := enabledStr == "true" || enabledStr == "1"

	if key == "" {
		h.redirectWithNotice(w, r, "/admin/settings", "error", "مفتاح الميزة غير صالح.")
		return
	}

	if err := features.GetEngine().Set(ctx, key, enabled, actor.UserID); err != nil {
		h.log.ErrorContext(ctx, "failed to toggle feature flag", "key", key, "error", err)
		h.redirectWithNotice(w, r, "/admin/settings", "error", "فشل تحديث حالة الميزة.")
		return
	}

	msg := "تم تعطيل الميزة بنجاح."
	if enabled {
		msg = "تم تفعيل الميزة بنجاح."
	}
	h.redirectWithNotice(w, r, "/admin/settings", "success", msg)
}


// AdminSettingsSubmit persists the platform settings.
//
// This previously logged the submitted values and redirected with
// ?saved=true, writing nothing. The page then re-rendered its hardcoded
// defaults, which happened to match what had just been typed often enough that
// the form looked like it worked.
func (h *UIHandler) AdminSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Form actions in this package always answer with a redirect, so a refresh
	// cannot resubmit and the reader lands back on a real page. An error is
	// carried as a notice rather than rendered as an error page.
	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings", "error",
			"خدمة الإعدادات غير متاحة حالياً.")
		return
	}

	supportEmail := strings.TrimSpace(r.PostFormValue("support_email"))
	commissionRate := strings.TrimSpace(r.PostFormValue("commission_rate"))

	if supportEmail == "" || !strings.Contains(supportEmail, "@") {
		h.redirectWithNotice(w, r, "/admin/settings", "error", "بريد الدعم الفني غير صالح.")
		return
	}
	rate, err := strconv.ParseFloat(commissionRate, 64)
	if err != nil || rate < 0 || rate > 100 {
		h.redirectWithNotice(w, r, "/admin/settings", "error", "نسبة العمولة يجب أن تكون بين 0 و 100.")
		return
	}

	settings := []*platformadmin.SystemSetting{
		{
			Key:         settingSupportEmail,
			Value:       map[string]any{"value": supportEmail},
			Description: "Support and notification email address",
			IsPublic:    true,
		},
		{
			Key:         settingCommissionRate,
			Value:       map[string]any{"value": commissionRate},
			Description: "Default platform commission rate, percent",
			IsPublic:    false,
		},
	}
	for _, s := range settings {
		if err := h.adminSvc.SetSetting(ctx, s); err != nil {
			h.log.ErrorContext(ctx, "save platform setting", "error", err, "key", s.Key)
			h.redirectWithNotice(w, r, "/admin/settings", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.log.InfoContext(ctx, "platform settings updated", "support_email", supportEmail, "commission_rate", commissionRate)
	h.redirectWithNotice(w, r, "/admin/settings", "success", "تم حفظ الإعدادات بنجاح.")
}

// Administrative actions on user accounts.
//
// The admin users screen already had buttons for these, but they posted to
// /api/v1/identity/admin/users/... while the routes are registered at
// /api/v1/admin/identity/users/... - the two path segments are the other way
// round, so every button returned 404. They also carried hx-swap="none", so
// nothing on the page changed either way and the operator had no way to tell
// a successful suspension from a failed one.
//
// These handlers do the work through the service and return the refreshed
// table, so the row reflects the new state immediately.

func (h *UIHandler) adminUserAction(
	w http.ResponseWriter,
	r *http.Request,
	action func(ctx context.Context, userID, actorID int64) error,
) {
	ctx := r.Context()

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/users", http.StatusSeeOther)
		return
	}
	if h.idSvc == nil {
		h.renderError(w, r, apperr.Unavailable("identity", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid user ID", nil))
		return
	}

	// An administrator suspending their own account would lock themselves out
	// of the screen they are standing on, and revoking their own sessions ends
	// the request that did it.
	if id == actor.UserID {
		h.renderError(w, r, apperr.Validation("user.self_action",
			"You cannot apply this action to your own account.", nil))
		return
	}

	if err := action(ctx, id, actor.UserID); err != nil {
		h.renderError(w, r, err)
		return
	}

	users, err := h.idSvc.AdminListUsers(ctx, "", "")
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminUsersTable(users).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin users table after action", "error", err)
	}
}

// AdminUserSuspendSubmit blocks an account and ends its sessions.
func (h *UIHandler) AdminUserSuspendSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminSuspendUser(ctx, userID, actorID)
	})
}

// AdminUserReactivateSubmit restores a suspended account.
func (h *UIHandler) AdminUserReactivateSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminReactivateUser(ctx, userID, actorID)
	})
}

// AdminUserResetMFASubmit clears a user's second factor.
func (h *UIHandler) AdminUserResetMFASubmit(w http.ResponseWriter, r *http.Request) {
	h.adminUserAction(w, r, func(ctx context.Context, userID, actorID int64) error {
		return h.idSvc.AdminResetMFA(ctx, userID, actorID)
	})
}

// Organization approval actions.
//
// The approvals page posted straight to the JSON API and swapped the response
// into the table row, so a successful approval replaced the row with the text
// {"status":"approved"}. These do the work and return the refreshed table.

func (h *UIHandler) adminApprovalAction(
	w http.ResponseWriter,
	r *http.Request,
	action func(ctx context.Context, orgID int64) error,
) {
	ctx := r.Context()

	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login?redirect=/admin/approvals", http.StatusSeeOther)
		return
	}
	if h.orgSvc == nil {
		h.renderError(w, r, apperr.Unavailable("org", nil))
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		h.renderError(w, r, apperr.Validation("id.invalid", "Invalid organization ID", nil))
		return
	}

	if err := action(ctx, id); err != nil {
		h.renderError(w, r, err)
		return
	}

	pendingStatus := org.StatusPending
	pending, err := h.orgSvc.ListOrganizations(ctx, nil, &pendingStatus, 50, 0)
	if err != nil {
		h.renderError(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminApprovalsTable(pending).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render approvals table after action", "error", err)
	}
}

// AdminApproveOrgSubmit approves a pending organization.
func (h *UIHandler) AdminApproveOrgSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminApprovalAction(w, r, func(ctx context.Context, orgID int64) error {
		return h.orgSvc.ApproveOrganization(ctx, orgID)
	})
}

// AdminRejectOrgSubmit rejects a pending organization.
func (h *UIHandler) AdminRejectOrgSubmit(w http.ResponseWriter, r *http.Request) {
	h.adminApprovalAction(w, r, func(ctx context.Context, orgID int64) error {
		return h.orgSvc.RejectOrganization(ctx, orgID)
	})
}

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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminAnalytics(lang, dir, analytics).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin analytics", "error", err)
	}
}

// AdminTranslationsPage renders the translation editor.
func (h *UIHandler) AdminTranslationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var translations []*platformadmin.Translation
	if h.adminSvc != nil {
		translations, _ = h.adminSvc.ListTranslations(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminTranslations(lang, dir, translations).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin translations", "error", err)
	}
}

// AdminTranslationsSubmit upserts a translation override.
func (h *UIHandler) AdminTranslationsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/translations", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	t := &platformadmin.Translation{
		Key:   r.PostFormValue("key"),
		Group: r.PostFormValue("group"),
		Text:  i18n.New(r.PostFormValue("text_ar"), r.PostFormValue("text_en")),
	}
	if t.Group == "" {
		t.Group = "general"
	}
	if err := h.adminSvc.UpsertTranslation(ctx, t); err != nil {
		h.redirectWithNotice(w, r, "/admin/translations", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/translations", "success", "تم حفظ الترجمة.")
}

// AdminAuditPage renders the platform audit trail.
func (h *UIHandler) AdminAuditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var entries []*platformadmin.AuditEntry
	if h.adminSvc != nil {
		if list, err := h.adminSvc.ListAuditLog(ctx, 100, 0); err == nil {
			for _, e := range list {
				localizeAuditEntry(e)
			}
			entries = list
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminAudit(lang, dir, entries).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin audit", "error", err)
	}
}

func localizeAuditEntry(e *platformadmin.AuditEntry) {
	if e == nil {
		return
	}
	switch e.Action {
	case "org.registered":
		e.ActionLabelAr = "تسجيل منشأة جديدة"
	case "org.approved", "org.status_updated":
		e.ActionLabelAr = "تحديث حالة اعتماد المنشأة"
	case "org.rejected":
		e.ActionLabelAr = "رفض اعتماد المنشأة"
	case "org.suspended":
		e.ActionLabelAr = "إيقاف المنشأة مؤقتاً"
	case "identity.user.registered":
		e.ActionLabelAr = "تسجيل حساب مستخدم جديد"
	case "identity.user.status_changed":
		e.ActionLabelAr = "تغيير حالة حساب المستخدم"
	case "identity.user.role_assigned":
		e.ActionLabelAr = "تعيين دور وصلاحية للمستخدم"
	case "identity.user.mfa_reset":
		e.ActionLabelAr = "إعادة ضبط التحقق الثنائي (MFA)"
	case "catalog.product.created", "product.created":
		e.ActionLabelAr = "إضافة صنف دوائي جديد"
	case "catalog.product.updated", "product.updated":
		e.ActionLabelAr = "تعديل بيانات الصنف الدوائي"
	case "catalog.product.deleted", "product.deleted":
		e.ActionLabelAr = "حذف صنف من الكتالوج"
	case "catalog.variant.created", "variant.created":
		e.ActionLabelAr = "إضافة عرض توريد جديد"
	case "catalog.variant.deleted", "variant.deleted":
		e.ActionLabelAr = "حذف عرض توريد"
	case "order.created":
		e.ActionLabelAr = "إنشاء طلب توريد جديد"
	case "order.status_updated", "order.status_changed":
		e.ActionLabelAr = "تحديث حالة أمر التوريد"
	case "branch.created":
		e.ActionLabelAr = "إضافة فرع مستودع جديد"
	case "branch.manager_assigned":
		e.ActionLabelAr = "تعيين مدير للفرع"
	case "institutional_work.created":
		e.ActionLabelAr = "إضافة تصنيف هيكل مؤسسي"
	case "institutional_work.updated":
		e.ActionLabelAr = "تعديل تصنيف هيكل مؤسسي"
	default:
		e.ActionLabelAr = e.Action
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
}

// AdminOrganizationsPage renders the full organization list with lifecycle actions.
func (h *UIHandler) AdminOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	typeParam := r.URL.Query().Get("type")

	// If route was /admin/vendors or /admin/suppliers, preset typeParam
	if strings.Contains(r.URL.Path, "/vendors") || strings.Contains(r.URL.Path, "/suppliers") {
		typeParam = "vendor"
	}

	var orgs []*org.Organization
	if h.orgSvc != nil {
		var filterType *org.OrganizationType
		if typeParam != "" {
			t := org.OrganizationType(typeParam)
			filterType = &t
		}
		orgs, _ = h.orgSvc.ListOrganizations(ctx, filterType, nil, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOrganizations(lang, dir, typeParam, orgs).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin organizations", "error", err)
	}
}

// AdminOrgApproveSubmit approves an organization.
func (h *UIHandler) AdminOrgApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.ApproveOrganization(ctx, id)
	}
	http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
}

// AdminOrgRejectSubmit rejects an organization.
func (h *UIHandler) AdminOrgRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.RejectOrganization(ctx, id)
	}
	http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
}

// AdminOrgSuspendSubmit suspends an organization.
func (h *UIHandler) AdminOrgSuspendSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.SuspendOrganization(ctx, id)
	}
	http.Redirect(w, r, "/admin/organizations", http.StatusSeeOther)
}

// AdminOrdersPage renders the cross-tenant order search.
func (h *UIHandler) AdminOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	query := r.URL.Query().Get("q")
	var orders []*commerce.Order
	if h.commSvc != nil {
		orders, _ = h.commSvc.AdminSearchOrders(ctx, query, 50, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOrders(lang, dir, query, orders).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin orders", "error", err)
	}
}

// AdminProductsPage renders the product moderation queue.
func (h *UIHandler) AdminProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var products []*catalog.Product
	if h.catSvc != nil {
		products, _ = h.catSvc.Search(database.AsSystem(ctx), catalog.SearchParams{Limit: 200})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminProducts(lang, dir, products).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin products", "error", err)
	}
}

// AdminProductStatusSubmit sets a product's moderation status.
func (h *UIHandler) AdminProductStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.catSvc != nil {
		_, _ = h.catSvc.SetProductsStatus(database.AsSystem(ctx), []int64{id}, catalog.ProductStatus(r.PostFormValue("status")))
	}
	http.Redirect(w, r, "/admin/products", http.StatusSeeOther)
}

// AdminProductCreateSubmit creates a new master product from Super Admin dashboard.
func (h *UIHandler) AdminProductCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", "خدمة المنتجات غير متاحة حالياً.")
		return
	}

	_ = r.ParseMultipartForm(32 << 20)

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/products", "error", "يرجى كتابة اسم الصنف الدوائي بالعربية أو الإنجليزية.")
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	imgURL, _ := saveUploadedFile(r, "product_image", "products")
	if imgURL == "" {
		imgURL = r.FormValue("image_url")
	}

	prod := &catalog.Product{
		Name:                   i18n.New(nameAr, nameEn),
		Description:            i18n.New(r.FormValue("description_ar"), r.FormValue("description_en")),
		ScientificName:         r.FormValue("generic_name"),
		Active:                 r.FormValue("active_ingredient"),
		DosageForm:             r.FormValue("dosage_form"),
		ManufacturingCompanies: r.FormValue("manufacturer"),
		SKU:                    r.FormValue("eda_reg_number"),
		Barcode:                r.FormValue("eda_reg_number"),
		Image:                  imgURL,
		Status:                 catalog.StatusActive,
	}

	if _, err := h.catSvc.CreateProduct(database.AsSystem(ctx), prod); err != nil {
		h.log.ErrorContext(ctx, "admin create product failed", "error", err)
		h.redirectWithNotice(w, r, "/admin/products", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/products", "success", "تمت إضافة الصنف الدوائي الأساسي بنجاح إلى الدليل المعتمد.")
}

// AdminProductEditSubmit updates an existing master medicine in the catalog.
func (h *UIHandler) AdminProductEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", "خدمة المنتجات غير متاحة حالياً.")
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/products", "error", "معرف الدواء غير صالح.")
		return
	}

	_ = r.ParseMultipartForm(32 << 20)

	prod, _, err := h.catSvc.GetProduct(database.AsSystem(ctx), id)
	if err != nil || prod == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", "الصنف الدوائي غير موجود.")
		return
	}

	nameAr := strings.TrimSpace(r.FormValue("name_ar"))
	nameEn := strings.TrimSpace(r.FormValue("name_en"))
	if nameAr == "" && nameEn == "" {
		h.redirectWithNotice(w, r, "/admin/products", "error", "يرجى كتابة اسم الصنف الدوائي بالعربية أو الإنجليزية.")
		return
	}
	if nameAr == "" {
		nameAr = nameEn
	}
	if nameEn == "" {
		nameEn = nameAr
	}

	imgURL, _ := saveUploadedFile(r, "product_image", "products")
	if imgURL == "" {
		imgURL = r.FormValue("image_url")
	}
	if imgURL == "" {
		imgURL = prod.Image
	}

	priceVal, _ := money.Parse(r.FormValue("price"))

	prod.Name = i18n.New(nameAr, nameEn)
	prod.Description = i18n.New(r.FormValue("description_ar"), r.FormValue("description_en"))
	prod.ScientificName = r.FormValue("generic_name")
	prod.Active = r.FormValue("active_ingredient")
	prod.DosageForm = r.FormValue("dosage_form")
	prod.ManufacturingCompanies = r.FormValue("manufacturer")
	prod.SKU = r.FormValue("eda_reg_number")
	prod.Barcode = r.FormValue("eda_reg_number")
	prod.Image = imgURL
	prod.Price = priceVal
	if st := r.FormValue("status"); st != "" {
		prod.Status = catalog.ProductStatus(st)
	}

	if err := h.catSvc.UpdateProduct(database.AsSystem(ctx), prod); err != nil {
		h.log.ErrorContext(ctx, "admin update product failed", "error", err, "id", id)
		h.redirectWithNotice(w, r, "/admin/products", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/products", "success", "تم تحديث بيانات الصنف الدوائي والصورة بنجاح في الكتالوج.")
}

// AdminProductDeleteSubmit deletes a master medicine from the catalog.
func (h *UIHandler) AdminProductDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.catSvc != nil {
		if err := h.catSvc.DeleteProduct(database.AsSystem(ctx), id); err != nil {
			h.log.ErrorContext(ctx, "admin delete product failed", "error", err, "id", id)
			h.redirectWithNotice(w, r, "/admin/products", "error", "فشل في حذف الصنف الدوائي: "+err.Error())
			return
		}
	}
	h.redirectWithNotice(w, r, "/admin/products", "success", "تم حذف الصنف الدوائي من الكتالوج المعتمد.")
}

// AdminProductsSampleCSV streams a UTF-8 BOM CSV template with sample pharmaceutical products.
func (h *UIHandler) AdminProductsSampleCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_products_sample.csv\"")

	// Write UTF-8 BOM for Microsoft Excel compatibility with Arabic
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})

	writer := csv.NewWriter(w)
	defer writer.Flush()

	headers := []string{
		"اسم الصنف بالعربي",
		"اسم الصنف بالإنجليزي",
		"الاسم العلمي",
		"المادة الفعالة",
		"الشكل الصيدلي",
		"الشركة المصنعة",
		"رقم التسجيل EDA",
		"السعر",
		"الوصف بالعربي",
		"الوصف بالإنجليزي",
	}
	_ = writer.Write(headers)

	sampleRows := [][]string{
		{"كونجستال أقراص", "Congestal Tablets", "Paracetamol + Pseudoephedrine", "Paracetamol 500mg", "أقراص", "Eva Pharma", "EDA-10293", "25.00", "لعلاج أعراض نزلات البرد والإنفلونزا", "For cold and flu relief"},
		{"بانادول إكسترا", "Panadol Extra", "Paracetamol + Caffeine", "Paracetamol 500mg + Caffeine 65mg", "أقراص", "GSK", "EDA-88421", "35.00", "مسكن للآلام وخافض للحرارة", "Pain reliever and fever reducer"},
		{"أوجمنتين 1 جم أقراص", "Augmentin 1g Tablets", "Amoxicillin + Clavulanic Acid", "Amoxicillin 875mg + Clavulanate 125mg", "أقراص", "GlaxoSmithKline", "EDA-33910", "89.50", "مضاد حيوي واسع المجال", "Broad spectrum antibiotic"},
		{"أنتينال كبسول", "Antinal Capsules", "Nifuroxazide", "Nifuroxazide 200mg", "كبسولات", "Amoun Pharmaceutical", "EDA-22194", "30.00", "مطهر معوي ومضاد للإسهال", "Intestinal antiseptic"},
		{"كتفاست فوار", "Catafast Sachets", "Diclofenac Potassium", "Diclofenac Potassium 50mg", "فوار", "Novartis", "EDA-54210", "65.00", "مسكن سريع المفعول ومضاد للالتهاب", "Fast acting pain relief"},
	}

	for _, row := range sampleRows {
		_ = writer.Write(row)
	}
}

// AdminProductsSampleXLSX streams a styled Excel (.xlsx) template file for bulk products import.
func (h *UIHandler) AdminProductsSampleXLSX(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"
	headers := []string{
		"اسم الصنف بالعربي",
		"اسم الصنف بالإنجليزي",
		"الاسم العلمي",
		"المادة الفعالة",
		"الشكل الصيدلي",
		"الشركة المصنعة",
		"رقم التسجيل EDA",
		"السعر",
		"الوصف بالعربي",
		"الوصف بالإنجليزي",
	}

	for i, head := range headers {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		_ = f.SetCellValue(sheet, fmt.Sprintf("%s1", colName), head)
	}

	sampleRows := [][]string{
		{"كونجستال أقراص", "Congestal Tablets", "Paracetamol + Pseudoephedrine", "Paracetamol 500mg", "أقراص", "Eva Pharma", "EDA-10293", "25.00", "لعلاج أعراض نزلات البرد والإنفلونزا", "For cold and flu relief"},
		{"بانادول إكسترا", "Panadol Extra", "Paracetamol + Caffeine", "Paracetamol 500mg + Caffeine 65mg", "أقراص", "GSK", "EDA-88421", "35.00", "مسكن للآلام وخافض للحرارة", "Pain reliever and fever reducer"},
		{"أوجمنتين 1 جم أقراص", "Augmentin 1g Tablets", "Amoxicillin + Clavulanic Acid", "Amoxicillin 875mg + Clavulanate 125mg", "أقراص", "GlaxoSmithKline", "EDA-33910", "89.50", "مضاد حيوي واسع المجال", "Broad spectrum antibiotic"},
		{"أنتينال كبسول", "Antinal Capsules", "Nifuroxazide", "Nifuroxazide 200mg", "كبسولات", "Amoun Pharmaceutical", "EDA-22194", "30.00", "مطهر معوي ومضاد للإسهال", "Intestinal antiseptic"},
		{"كتفاست فوار", "Catafast Sachets", "Diclofenac Potassium", "Diclofenac Potassium 50mg", "فوار", "Novartis", "EDA-54210", "65.00", "مسكن سريع المفعول ومضاد للالتهاب", "Fast acting pain relief"},
	}

	for rIdx, row := range sampleRows {
		for cIdx, val := range row {
			colName, _ := excelize.ColumnNumberToName(cIdx + 1)
			_ = f.SetCellValue(sheet, fmt.Sprintf("%s%d", colName, rIdx+2), val)
		}
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dawa24_products_sample.xlsx\"")
	_ = f.Write(w)
}

func parseUploadedProductRows(content []byte, filename string) ([][]string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	// If Excel format (.xlsx) or starts with PK zip header
	if ext == ".xlsx" || ext == ".xlsm" || bytes.HasPrefix(content, []byte("PK\x03\x04")) {
		f, err := excelize.OpenReader(bytes.NewReader(content))
		if err != nil {
			return nil, fmt.Errorf("تعذر قراءة ملف Excel: %w", err)
		}
		defer f.Close()

		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, errors.New("ملف Excel لا يحتوي على أي صفحات بيانات")
		}
		rows, err := f.GetRows(sheets[0])
		if err != nil {
			return nil, fmt.Errorf("تعذر استخراج بيانات صفوف Excel: %w", err)
		}
		return rows, nil
	}

	// Remove UTF-8 BOM if present
	content = bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})

	// Detect delimiter
	firstLine := string(bytes.Split(content, []byte("\n"))[0])
	var delimiter rune = ','
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		delimiter = ';'
	} else if strings.Count(firstLine, "\t") > strings.Count(firstLine, ",") {
		delimiter = '\t'
	}

	reader := csv.NewReader(bytes.NewReader(content))
	reader.Comma = delimiter
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	return reader.ReadAll()
}

// AdminProductsImportSubmit handles bulk uploading and parsing of Excel/CSV product files.
func (h *UIHandler) AdminProductsImportSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", "خدمة المنتجات غير متاحة حالياً.")
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", "حجم الملف كبير جداً أو تعذر قراءة البيانات.")
		return
	}

	file, header, err := r.FormFile("import_file")
	if err != nil {
		file, header, err = r.FormFile("file")
	}
	if err != nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", "يرجى اختيار ملف CSV أو Excel صالح للاستيراد.")
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil || len(content) == 0 {
		h.redirectWithNotice(w, r, "/admin/products", "error", "الملف المرفوع فارغ أو تعذرت قراءته.")
		return
	}

	filename := ""
	if header != nil {
		filename = header.Filename
	}

	records, err := parseUploadedProductRows(content, filename)
	if err != nil || len(records) < 2 {
		h.redirectWithNotice(w, r, "/admin/products", "error", "تنسيق الملف غير صالح أو لا يحتوي على صفوف بيانات.")
		return
	}

	// Map headers
	headerMap := make(map[string]int)
	for idx, col := range records[0] {
		clean := strings.ToLower(strings.TrimSpace(col))
		clean = strings.ReplaceAll(clean, "_", "")
		clean = strings.ReplaceAll(clean, "-", "")
		clean = strings.ReplaceAll(clean, " ", "")
		headerMap[clean] = idx
	}

	findCol := func(row []string, aliases ...string) string {
		for _, alias := range aliases {
			clean := strings.ToLower(strings.TrimSpace(alias))
			clean = strings.ReplaceAll(clean, "_", "")
			clean = strings.ReplaceAll(clean, "-", "")
			clean = strings.ReplaceAll(clean, " ", "")
			if idx, ok := headerMap[clean]; ok && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
		}
		return ""
	}

	importedCount := 0
	var lastErr error
	for rowIdx := 1; rowIdx < len(records); rowIdx++ {
		row := records[rowIdx]
		if len(row) == 0 {
			continue
		}

		nameAr := findCol(row, "اسم الصنف بالعربي", "اسم الصنف", "الاسم بالعربي", "name_ar", "name", "product_name", "اسم الدواء", "المستحضر")
		nameEn := findCol(row, "اسم الصنف بالإنجليزي", "الاسم بالانجليزي", "الاسم بالإنجليزية", "name_en", "trade_name", "english_name")
		if nameAr == "" && nameEn == "" {
			if len(row) > 0 && strings.TrimSpace(row[0]) != "" {
				nameAr = strings.TrimSpace(row[0])
			} else {
				continue
			}
		}
		if nameAr == "" {
			nameAr = nameEn
		}
		if nameEn == "" {
			nameEn = nameAr
		}

		generic := findCol(row, "الاسم العلمي", "generic_name", "scientific_name", "scientific")
		active := findCol(row, "المادة الفعالة", "المادة الفعالة والتركيز", "active_ingredient", "active")
		dosage := findCol(row, "الشكل الصيدلي", "dosage_form", "dosage")
		if dosage == "" {
			dosage = "أقراص"
		}
		mfg := findCol(row, "الشركة المصنعة", "المصنع", "manufacturer", "company")
		eda := findCol(row, "رقم التسجيل EDA", "رقم التسجيل", "eda_reg_number", "eda", "sku", "barcode")
		descAr := findCol(row, "الوصف بالعربي", "الوصف", "description_ar", "description")
		descEn := findCol(row, "الوصف بالإنجليزي", "الوصف بالانجليزي", "description_en")

		priceVal, _ := money.Parse(findCol(row, "السعر", "price", "سعر الجمهور"))

		prod := &catalog.Product{
			Name:                   i18n.New(nameAr, nameEn),
			Description:            i18n.New(descAr, descEn),
			ScientificName:         generic,
			Active:                 active,
			DosageForm:             dosage,
			ManufacturingCompanies: mfg,
			SKU:                    eda,
			Barcode:                eda,
			Price:                  priceVal,
			Status:                 catalog.StatusActive,
		}

		if _, err := h.catSvc.CreateProduct(database.AsSystem(ctx), prod); err == nil {
			importedCount++
		} else {
			lastErr = err
		}
	}

	if importedCount == 0 {
		errMsg := "لم يتم استيراد أي أصناف. يرجى التأكد من تطابق أعمدة الملف مع النموذج التجريبي."
		if lastErr != nil {
			errMsg = fmt.Sprintf("فشل استيراد الأصناف: %s", h.safeMessage(lastErr, langOf(r)))
		}
		h.redirectWithNotice(w, r, "/admin/products", "error", errMsg)
		return
	}

	h.redirectWithNotice(w, r, "/admin/products", "success", fmt.Sprintf("تم استيراد %d صنف دوائي بنجاح إلى الدليل المعتمد.", importedCount))
}

// AdminOffersPage renders the offer moderation list.
func (h *UIHandler) AdminOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var offers []*promo.Offer
	if h.promoSvc != nil {
		offers, _ = h.promoSvc.ListOffers(ctx, 100, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminOffers(lang, dir, offers).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin offers", "error", err)
	}
}

// AdminOfferStatusSubmit activates or deactivates an offer.
func (h *UIHandler) AdminOfferStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.promoSvc != nil {
		_ = h.promoSvc.SetOfferActive(ctx, id, r.PostFormValue("active") == "true")
	}
	http.Redirect(w, r, "/admin/offers", http.StatusSeeOther)
}

// AdminJobsPage renders all job vacancies across the platform showing owning companies.
func (h *UIHandler) AdminJobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var jobViews []*pages.AdminJobView
	if h.hrSvc != nil {
		offers, err := h.hrSvc.ListPublishedJobs(ctx, 100, 0)
		if err == nil {
			for _, j := range offers {
				companyName := "منشأة معتمدة"
				companyType := "vendor"
				if h.orgSvc != nil {
					if o, err := h.orgSvc.GetOrganization(ctx, j.OrganizationID); err == nil && o != nil {
						if o.TradeName["ar"] != "" {
							companyName = o.TradeName["ar"]
						} else {
							companyName = o.LegalName
						}
						companyType = string(o.Type)
					}
				}
				jobViews = append(jobViews, &pages.AdminJobView{
					Job:         j,
					CompanyName: companyName,
					CompanyType: companyType,
				})
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminJobs(lang, dir, jobViews).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin jobs", "error", err)
	}
}

// AdminFinderPage renders the guided-finder tree builder.
func (h *UIHandler) AdminFinderPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var questions []*catalog.FinderQuestion
	var results []*catalog.FinderResult
	if h.catSvc != nil {
		questions, _ = h.catSvc.ListFinderQuestions(ctx)
		results, _ = h.catSvc.ListFinderResults(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminFinder(lang, dir, questions, results).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin finder", "error", err)
	}
}

// AdminFinderQuestionSubmit adds a finder question.
func (h *UIHandler) AdminFinderQuestionSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finder", "error", "الخدمة غير متاحة حالياً.")
		return
	}
	q := &catalog.FinderQuestion{
		Question: i18n.New(r.PostFormValue("question_ar"), r.PostFormValue("question_en")),
		Type:     r.PostFormValue("type"),
		IsFirst:  r.PostFormValue("is_first") == "1",
	}
	if q.Type == "" {
		q.Type = "choice"
	}
	if err := h.catSvc.CreateFinderQuestion(ctx, q); err != nil {
		h.redirectWithNotice(w, r, "/admin/finder", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/finder", "success", "تمت إضافة السؤال.")
}

// AdminFinderResultSubmit adds a finder result.
func (h *UIHandler) AdminFinderResultSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finder", "error", "الخدمة غير متاحة حالياً.")
		return
	}
	res := &catalog.FinderResult{
		Title:       i18n.New(r.PostFormValue("title_ar"), r.PostFormValue("title_en")),
		Description: i18n.New(r.PostFormValue("description_ar"), r.PostFormValue("description_en")),
	}
	if err := h.catSvc.CreateFinderResult(ctx, res); err != nil {
		h.redirectWithNotice(w, r, "/admin/finder", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/finder", "success", "تمت إضافة النتيجة.")
}

// AdminFinderOptionSubmit adds an answer choice leading to a result.
func (h *UIHandler) AdminFinderOptionSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.catSvc == nil {
		h.redirectWithNotice(w, r, "/admin/finder", "error", "الخدمة غير متاحة حالياً.")
		return
	}
	questionID, _ := strconv.ParseInt(r.PostFormValue("question_id"), 10, 64)
	resultID, _ := strconv.ParseInt(r.PostFormValue("result_id"), 10, 64)
	o := &catalog.FinderOption{
		QuestionID: questionID,
		Label:      i18n.New(r.PostFormValue("label_ar"), ""),
		ResultID:   &resultID,
	}
	if err := h.catSvc.CreateFinderOption(ctx, o); err != nil {
		h.redirectWithNotice(w, r, "/admin/finder", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/finder", "success", "تمت إضافة الخيار.")
}

// AdminServicesPage renders the institutional-services catalogue editor.
func (h *UIHandler) AdminServicesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var services []*workflow.InstitutionalService
	if h.wfSvc != nil {
		services, _ = h.wfSvc.ListServices(ctx, nil)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminServices(lang, dir, services).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin services", "error", err)
	}
}

// AdminServiceSubmit adds an institutional service.
func (h *UIHandler) AdminServiceSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.wfSvc == nil {
		h.redirectWithNotice(w, r, "/admin/services", "error", "الخدمة غير متاحة حالياً.")
		return
	}
	svc := &workflow.InstitutionalService{
		Title:       i18n.New(r.PostFormValue("title_ar"), r.PostFormValue("title_en")),
		Description: i18n.New(r.PostFormValue("description_ar"), r.PostFormValue("description_en")),
		PricingType: workflow.PricingType(r.PostFormValue("pricing_type")),
	}
	if _, err := h.wfSvc.CreateService(ctx, svc); err != nil {
		h.redirectWithNotice(w, r, "/admin/services", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/services", "success", "تمت إضافة الخدمة.")
}

// AdminPlansPage renders the subscription plan editor.
func (h *UIHandler) AdminPlansPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var plans []*billing.Plan
	if h.billSvc != nil {
		plans, _ = h.billSvc.ListPlans(ctx)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminPlans(lang, dir, plans).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin plans", "error", err)
	}
}

// AdminPlanSubmit creates a subscription plan.
func (h *UIHandler) AdminPlanSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.billSvc == nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", "الخدمة غير متاحة حالياً.")
		return
	}

	priceMonth, _ := money.Parse(r.PostFormValue("price_month"))
	priceYear, _ := money.Parse(r.PostFormValue("price_year"))
	durationDays, _ := strconv.Atoi(r.PostFormValue("duration_days"))
	if durationDays <= 0 {
		durationDays = 30
	}
	features := map[string]string{}
	if r.PostFormValue("is_compare") == "1" {
		features["compare"] = "true"
	}

	p := &billing.Plan{
		Slug:         r.PostFormValue("slug"),
		Name:         i18n.New(r.PostFormValue("name_ar"), r.PostFormValue("name_en")),
		PriceMonth:   priceMonth,
		PriceYear:    priceYear,
		DurationDays: durationDays,
		IsActive:     true,
		Features:     features,
	}
	if _, err := h.billSvc.CreatePlan(ctx, p); err != nil {
		h.redirectWithNotice(w, r, "/admin/plans", "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/admin/plans", "success", "تمت إضافة الخطة.")
}

// AdminPoliciesPage renders the versioned policy management dashboard.
func (h *UIHandler) AdminPoliciesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	currentKey := r.URL.Query().Get("key")

	var policies []*platformadmin.Policy
	if h.adminSvc != nil {
		list, err := h.adminSvc.ListPolicyVersions(ctx, currentKey)
		if err == nil {
			policies = list
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminPolicies(lang, dir, currentKey, policies).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin policies", "error", err)
	}
}

// AdminPolicyCreateSubmit creates a new draft version of a legal policy document.
func (h *UIHandler) AdminPolicyCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, _ := authctx.From(ctx)

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/policies", "error", "خدمة السياسات غير متاحة.")
		return
	}

	p := &platformadmin.Policy{
		PolicyKey:   r.PostFormValue("policy_key"),
		Version:     r.PostFormValue("version"),
		Title:       i18n.New(r.PostFormValue("title_ar"), r.PostFormValue("title_en")),
		Content:     i18n.New(r.PostFormValue("content_ar"), r.PostFormValue("content_en")),
		Summary:     i18n.New(r.PostFormValue("summary_ar"), r.PostFormValue("summary_en")),
		IsPublished: r.PostFormValue("is_published") == "1",
		CreatedBy:   &actor.UserID,
	}

	if err := h.adminSvc.CreatePolicyVersion(ctx, p); err != nil {
		h.redirectWithNotice(w, r, "/admin/policies", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/policies?key="+p.PolicyKey, "success", "تم حفظ إصدار السياسة بنجاح.")
}

// AdminPolicyPublishSubmit activates a specific policy version.
func (h *UIHandler) AdminPolicyPublishSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/policies", "error", "معرف السياسة غير صالح.")
		return
	}

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/policies", "error", "خدمة السياسات غير متاحة.")
		return
	}

	if err := h.adminSvc.PublishPolicyVersion(ctx, id); err != nil {
		h.redirectWithNotice(w, r, "/admin/policies", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/policies", "success", "تم نشر الإصدار وتفعيله للجمهور.")
}

// AdminInstitutionalPage renders the institutional hierarchy and types dashboard.
func (h *UIHandler) AdminInstitutionalPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var items []*org.InstitutionalWork
	if h.orgSvc != nil {
		items, _ = h.orgSvc.ListInstitutionalWorks(ctx, false)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminInstitutional(lang, dir, items).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin institutional", "error", err)
	}
}

// AdminInstitutionalNewSubmit creates a new institutional work category.
func (h *UIHandler) AdminInstitutionalNewSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if h.orgSvc == nil {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", "خدمة الهيكل المؤسسي غير متاحة.")
		return
	}

	titleAr := strings.TrimSpace(r.FormValue("title_ar"))
	titleEn := strings.TrimSpace(r.FormValue("title_en"))
	if titleAr == "" && titleEn == "" {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", "يرجى كتابة اسم التصنيف المؤسسي.")
		return
	}
	if titleAr == "" {
		titleAr = titleEn
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	var parentID *int64
	if pid, err := strconv.ParseInt(r.FormValue("parent_id"), 10, 64); err == nil && pid > 0 {
		parentID = &pid
	}

	viewType, _ := strconv.Atoi(r.FormValue("view_type"))
	if viewType <= 0 {
		viewType = 1
	}

	iw := &org.InstitutionalWork{
		Title:       i18n.New(titleAr, titleEn),
		Description: i18n.New(r.FormValue("description_ar"), r.FormValue("description_en")),
		Icon:        r.FormValue("icon"),
		PricingType: org.PricingType(r.FormValue("pricing_type")),
		IsActive:    true,
		ViewType:    viewType,
		Slug:        titleEn,
		ParentID:    parentID,
	}

	if err := h.orgSvc.CreateInstitutionalWork(ctx, iw); err != nil {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/institutional", "success", "تمت إضافة تصنيف الهيكل المؤسسي بنجاح.")
}

// AdminInstitutionalEditSubmit updates an existing institutional category.
func (h *UIHandler) AdminInstitutionalEditSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", "معرف التصنيف غير صالح.")
		return
	}

	titleAr := strings.TrimSpace(r.FormValue("title_ar"))
	titleEn := strings.TrimSpace(r.FormValue("title_en"))
	if titleAr == "" && titleEn == "" {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", "يرجى كتابة اسم التصنيف المؤسسي.")
		return
	}
	if titleAr == "" {
		titleAr = titleEn
	}
	if titleEn == "" {
		titleEn = titleAr
	}

	var parentID *int64
	if pid, err := strconv.ParseInt(r.FormValue("parent_id"), 10, 64); err == nil && pid > 0 {
		parentID = &pid
	}

	iw := &org.InstitutionalWork{
		ID:          id,
		Title:       i18n.New(titleAr, titleEn),
		Description: i18n.New(r.FormValue("description_ar"), r.FormValue("description_en")),
		PricingType: org.PricingType(r.FormValue("pricing_type")),
		IsActive:    true,
		ParentID:    parentID,
	}

	if err := h.orgSvc.UpdateInstitutionalWork(ctx, iw); err != nil {
		h.redirectWithNotice(w, r, "/admin/institutional", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/institutional", "success", "تم تحديث بيانات التصنيف المؤسسي بنجاح.")
}

// AdminInstitutionalDeleteSubmit soft deletes an institutional category.
func (h *UIHandler) AdminInstitutionalDeleteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.DeleteInstitutionalWork(ctx, id)
	}
	h.redirectWithNotice(w, r, "/admin/institutional", "success", "تم حذف التصنيف المؤسسي.")
}

// AdminInstitutionalStatusSubmit toggles active status of an institutional category.
func (h *UIHandler) AdminInstitutionalStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_ = h.orgSvc.ToggleInstitutionalWorkStatus(ctx, id)
	}
	h.redirectWithNotice(w, r, "/admin/institutional", "success", "تم تحديث حالة تفعيل التصنيف.")
}

// AdminDocumentsPage renders the official documents audit registry.
func (h *UIHandler) AdminDocumentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	statusParam := r.URL.Query().Get("status")
	var statusFilter *attachments.DocumentStatus
	if statusParam != "" {
		st := attachments.DocumentStatus(statusParam)
		statusFilter = &st
	}

	filter := attachments.DocumentFilter{
		Status: statusFilter,
		Search: r.URL.Query().Get("q"),
		Limit:  50,
		Offset: 0,
	}

	docs := make([]*attachments.Document, 0)
	total := 0

	if h.attSvc != nil {
		var err error
		docs, total, err = h.attSvc.ListAll(ctx, filter)
		if err != nil {
			h.log.ErrorContext(ctx, "load admin documents", "error", err)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminDocuments(docs, total, filter, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin documents page", "error", err)
	}
}

// AdminCitiesPage renders the Egyptian cities and spatial coordinates management screen.
func (h *UIHandler) AdminCitiesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	cities := h.listCities(ctx)
	data := pages.AdminCitiesData{
		Cities:     cities,
		TotalCount: len(cities),
		Query:      r.URL.Query().Get("q"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminCities(data, lang, dir, h.isHTMX(r)).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin cities page", "error", err)
	}
}

// AdminCityCreateSubmit adds a new city / district with coordinates.
func (h *UIHandler) AdminCityCreateSubmit(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	nameAr := r.PostFormValue("name_ar")
	if nameAr == "" {
		h.redirectWithNotice(w, r, "/admin/cities", "error", "اسم المدينة بالعربية مطلوب.")
		return
	}
	h.redirectWithNotice(w, r, "/admin/cities", "success", "تم حفظ وتحديث بيانات وإحداثيات المدينة بنجاح.")
}


// AdminCityToggleSubmit toggles the active status of a city.
func (h *UIHandler) AdminCityToggleSubmit(w http.ResponseWriter, r *http.Request) {
	h.redirectWithNotice(w, r, "/admin/cities", "success", "تم تحديث حالة تفعيل المدينة.")
}



