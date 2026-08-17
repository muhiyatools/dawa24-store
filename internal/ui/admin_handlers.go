package ui

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/attachments"
	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/modules/workflow"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
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
	}
	if h.commSvc != nil {
		if n, err := h.commSvc.CountOrders(ctx); err != nil {
			h.log.ErrorContext(ctx, "dashboard: count orders", "error", err)
		} else {
			stats.TotalOrders = n
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminDashboard(stats, lang, dir).Render(ctx, w); err != nil {
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
		entries, _ = h.adminSvc.ListAuditLog(ctx, 50, 0)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminAudit(lang, dir, entries).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin audit", "error", err)
	}
}

// AdminOrganizationsPage renders the full organization list with lifecycle actions.
func (h *UIHandler) AdminOrganizationsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	typeParam := r.URL.Query().Get("type")

	// If route was /admin/vendors or /admin/suppliers, preset typeParam
	if strings.Contains(r.URL.Path, "/vendors") || strings.Contains(r.URL.Path, "/suppliers") {
		typeParam = "supplier"
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
		products, _ = h.catSvc.Search(ctx, catalog.SearchParams{Limit: 100})
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
		_, _ = h.catSvc.SetProductsStatus(ctx, []int64{id}, catalog.ProductStatus(r.PostFormValue("status")))
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

	imgURL, _ := saveUploadedFile(r, "product_image", "products")
	if imgURL == "" {
		imgURL = r.PostFormValue("image_url")
	}

	prod := &catalog.Product{
		Name:                   i18n.New(r.PostFormValue("name_ar"), r.PostFormValue("name_en")),
		Description:            i18n.New(r.PostFormValue("description_ar"), r.PostFormValue("description_en")),
		ScientificName:         r.PostFormValue("generic_name"),
		Active:                 r.PostFormValue("active_ingredient"),
		DosageForm:             r.PostFormValue("dosage_form"),
		ManufacturingCompanies: r.PostFormValue("manufacturer"),
		Image:                  imgURL,
		Status:                 catalog.StatusActive,
	}

	if _, err := h.catSvc.CreateProduct(ctx, prod); err != nil {
		h.redirectWithNotice(w, r, "/admin/products", "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, "/admin/products", "success", "تمت إضافة الصنف الدوائي الأساسي بنجاح إلى الدليل المعتمد.")
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
				companyType := "supplier"
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.AdminDocuments(docs, total, filter, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render admin documents page", "error", err)
	}
}


