package ui

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	platformadmin "github.com/muhiya/dawa24-store/internal/modules/platform_admin"
	"github.com/muhiya/dawa24-store/internal/modules/promo"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"

	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// AdminOrdersPage renders the cross-tenant order search and procurement tabs.
func (h *UIHandler) AdminOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	query := r.URL.Query().Get("q")
	tab := r.URL.Query().Get("tab")
	if tab == "" {
		tab = "all"
	}

	var orders []*commerce.Order
	var directOrders []*commerce.Order
	var negOrders []*commerce.Order
	if h.commSvc != nil {
		orders, _ = h.commSvc.AdminSearchOrders(ctx, query, 200, 0)
		for _, o := range orders {
			if o.IsNegotiation {
				negOrders = append(negOrders, o)
			} else {
				directOrders = append(directOrders, o)
			}
		}
	}

	data := pages.AdminOrdersData{
		ActiveTab:         tab,
		Query:             query,
		Orders:            orders,
		DirectOrders:      directOrders,
		NegotiationOrders: negOrders,
	}

	h.renderPage(ctx, w, "render admin orders", pages.AdminOrdersHub(data, lang, dir))
}

// AdminOffersPage renders the special supplier bundle offers moderation list.
func (h *UIHandler) AdminOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	statusFilter := r.URL.Query().Get("status")

	var allOffers []*promo.SpecialOffer
	if h.promoSvc != nil {
		allOffers, _ = h.promoSvc.ListAllSpecialOffers(ctx, 200, 0)
	}

	var filteredOffers []*promo.SpecialOffer
	for _, o := range allOffers {
		if o == nil {
			continue
		}
		if statusFilter == "" || statusFilter == "all" {
			filteredOffers = append(filteredOffers, o)
		} else if statusFilter == "pending" && (o.AdminStatus == "pending" || o.AdminStatus == "") {
			filteredOffers = append(filteredOffers, o)
		} else if statusFilter == "active" && o.AdminStatus == "approved" && o.Status == "active" {
			filteredOffers = append(filteredOffers, o)
		} else if statusFilter == "rejected" && o.AdminStatus == "rejected" {
			filteredOffers = append(filteredOffers, o)
		} else if statusFilter == "draft" && (o.Status == "draft" || o.Status == "inactive") {
			filteredOffers = append(filteredOffers, o)
		}
	}

	data := pages.AdminOffersData{
		Offers:       filteredOffers,
		FilterStatus: statusFilter,
	}

	h.renderPage(ctx, w, "render admin offers", pages.AdminOffers(data, lang, dir))
}

// AdminOfferApproveSubmit approves a supplier special offer for marketplace publishing.
func (h *UIHandler) AdminOfferApproveSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/offers", "error", i18n.T(lang, "admin.commerce.invalid_offer_id"))
		return
	}

	actor, _ := authctx.From(ctx)
	if h.promoSvc != nil {
		if err := h.promoSvc.UpdateSpecialOfferAdminStatus(ctx, id, "approved", i18n.T(lang, "admin.commerce.approved_by_admin"), actor.UserID); err != nil {
			h.redirectWithNotice(w, r, "/admin/offers", "error", h.safeMessage(err, lang))
			return
		}
		_ = h.promoSvc.ToggleSpecialOfferStatus(ctx, id, true)
	}

	h.redirectWithNotice(w, r, "/admin/offers", "success", i18n.T(lang, "admin.commerce.offer_approved_success"))
}

// AdminOfferRejectSubmit rejects a supplier special offer.
func (h *UIHandler) AdminOfferRejectSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/offers", "error", i18n.T(lang, "admin.commerce.invalid_offer_id"))
		return
	}

	actor, _ := authctx.From(ctx)
	if h.promoSvc != nil {
		if err := h.promoSvc.UpdateSpecialOfferAdminStatus(ctx, id, "rejected", i18n.T(lang, "admin.commerce.rejected_by_admin"), actor.UserID); err != nil {
			h.redirectWithNotice(w, r, "/admin/offers", "error", h.safeMessage(err, lang))
			return
		}
		_ = h.promoSvc.ToggleSpecialOfferStatus(ctx, id, false)
	}

	h.redirectWithNotice(w, r, "/admin/offers", "success", i18n.T(lang, "admin.commerce.offer_rejected_success"))
}

// AdminOfferStatusSubmit activates or deactivates an offer.
func (h *UIHandler) AdminOfferStatusSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.promoSvc != nil {
		isActive := r.PostFormValue("active") == "true"
		_ = h.promoSvc.ToggleSpecialOfferStatus(ctx, id, isActive)
	}
	h.redirectWithNotice(w, r, "/admin/offers", "success", i18n.T(langOf(r), "admin.commerce.offer_status_toggled_success"))
}

// AdminJobsPage renders all job vacancies across the platform showing owning companies.
func (h *UIHandler) AdminJobsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	var jobViews []*pages.AdminJobView
	if h.hrSvc != nil {
		offers, err := h.hrSvc.ListPublishedJobs(ctx, 100, 0)
		if err != nil {
			h.log.WarnContext(ctx, "admin jobs: list published jobs", "error", err)
		} else {
			// Resolve every owning organization in one query. This used to be
			// a GetOrganization call inside the loop, so the page cost one
			// round trip per vacancy.
			orgs := map[int64]*org.Organization{}
			if h.orgSvc != nil && len(offers) > 0 {
				ids := make([]int64, 0, len(offers))
				for _, j := range offers {
					if j.OrganizationID > 0 {
						ids = append(ids, j.OrganizationID)
					}
				}
				resolved, orgErr := h.orgSvc.GetOrganizations(ctx, ids)
				if orgErr != nil {
					h.log.WarnContext(ctx, "admin jobs: resolve owning organizations", "error", orgErr)
				} else {
					orgs = resolved
				}
			}

			for _, j := range offers {
				companyName := i18n.T(lang, "admin.jobs_unknown_company")
				companyType := "vendor"
				if o := orgs[j.OrganizationID]; o != nil {
					if o.TradeName["ar"] != "" {
						companyName = o.TradeName["ar"]
					} else {
						companyName = o.LegalName
					}
					companyType = string(o.Type)
				}
				jobViews = append(jobViews, &pages.AdminJobView{
					Job:         j,
					CompanyName: companyName,
					CompanyType: companyType,
				})
			}
		}
	}

	h.renderPage(ctx, w, "render admin jobs", pages.AdminJobs(lang, dir, jobViews))
}

// AdminPolicyCreateSubmit creates a new draft version of a legal policy document.
func (h *UIHandler) AdminPolicyCreateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	actor, _ := authctx.From(ctx)

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", i18n.T(lang, "admin.commerce.policy_service_unavailable"))
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
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=policies&key="+p.PolicyKey, "success", i18n.T(lang, "admin.commerce.policy_saved_success"))
}

// AdminPolicyPublishSubmit activates a specific policy version.
func (h *UIHandler) AdminPolicyPublishSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", i18n.T(lang, "admin.commerce.invalid_policy_id"))
		return
	}

	if h.adminSvc == nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", i18n.T(lang, "admin.commerce.policy_service_unavailable"))
		return
	}

	if err := h.adminSvc.PublishPolicyVersion(ctx, id); err != nil {
		h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/admin/settings?tab=policies", "success", i18n.T(lang, "admin.commerce.policy_published_success"))
}
