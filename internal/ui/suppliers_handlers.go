package ui

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/platform/database"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// SuppliersPage renders the public supplier directory.
func (h *UIHandler) SuppliersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))

	data := pages.SupplierDirectoryData{
		Query: q,
	}
	if h.orgSvc != nil {
		sysCtx := database.AsSystem(ctx)
		typ := org.TypeVendor
		orgs, _ := h.orgSvc.ListOrganizations(sysCtx, &typ, nil, 100, 0)
		if len(orgs) == 0 {
			allOrgs, _ := h.orgSvc.ListOrganizations(sysCtx, nil, nil, 200, 0)
			for _, o := range allOrgs {
				if o != nil && (o.Type == org.TypeVendor || string(o.Type) == "supplier" || string(o.Type) == "distributor") {
					orgs = append(orgs, o)
				}
			}
		}

		var filtered []*org.Organization
		for _, o := range orgs {
			if o == nil {
				continue
			}
			// Only show approved or active suppliers, exclude rejected/suspended
			if o.Status == org.StatusRejected || o.Status == org.StatusSuspended {
				continue
			}
			if q != "" {
				nameAr := strings.ToLower(o.TradeName.Get(i18n.AR))
				nameEn := strings.ToLower(o.TradeName.Get(i18n.EN))
				legal := strings.ToLower(o.LegalName)
				cr := strings.ToLower(o.CommercialRegister)
				if !strings.Contains(nameAr, q) && !strings.Contains(nameEn, q) && !strings.Contains(legal, q) && !strings.Contains(cr, q) {
					continue
				}
			}
			filtered = append(filtered, o)
		}
		data.Suppliers = filtered
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SuppliersDirectory(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render suppliers directory", "error", err)
	}
}

// FollowedSuppliersPage renders the list of suppliers followed by the current user.
func (h *UIHandler) FollowedSuppliersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/suppliers/followed", http.StatusSeeOther)
		return
	}

	var suppliers []*org.Organization
	if h.orgSvc != nil {
		suppliers, _ = h.orgSvc.ListFollowedOrganizations(ctx, userID)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerFollowedSuppliers(suppliers, lang, dir).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render followed suppliers page", "error", err)
	}
}

// SupplierProfilePage renders a supplier's public profile: catalogue, reviews
// and policies.
func (h *UIHandler) SupplierProfilePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || h.orgSvc == nil {
		h.renderError(w, r, err)
		return
	}

	sysCtx := database.AsSystem(ctx)
	o, err := h.orgSvc.GetOrganization(sysCtx, id)
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	// Allow approved or active suppliers
	if o.Status == org.StatusRejected || o.Status == org.StatusSuspended {
		h.renderError(w, r, fmt.Errorf("المورد غير متاح حالياً"))
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	page := 1
	if pStr := r.URL.Query().Get("page"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			page = p
		}
	}
	limit := 24
	offset := (page - 1) * limit

	data := pages.SupplierProfileData{
		Org:         o,
		CurrentPage: page,
		SearchQuery: q,
	}

	if h.catSvc != nil {
		variants, total, err := h.catSvc.ListVariantsByOrganization(ctx, id, catalog.VariantSearchParams{
			Query:  q,
			Limit:  limit,
			Offset: offset,
		})
		if err == nil {
			data.Variants = variants
			data.TotalVariants = total
			if total > 0 {
				data.TotalPages = int(math.Ceil(float64(total) / float64(limit)))
			} else {
				data.TotalPages = 1
			}
		}
	}

	if h.promoSvc != nil {
		data.Sections, _ = h.promoSvc.ListHighlightSectionsByOrg(ctx, id)
	}
	if h.orgSvc != nil {
		data.Reviews, _ = h.orgSvc.ListReviews(ctx, id, 20, 0)
		data.Policies, _ = h.orgSvc.ListPolicies(ctx, id)
		if actor, ok := authctx.From(ctx); ok {
			data.IsFollowing, _ = h.orgSvc.IsFollowing(ctx, id, actor.UserID)
		}
	}
	data.ReviewCount = len(data.Reviews)
	if data.ReviewCount > 0 {
		var sum int
		for _, rv := range data.Reviews {
			sum += rv.Rating
		}
		data.Rating = float64(sum) / float64(data.ReviewCount)
	} else {
		data.Rating = 0
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.SupplierProfile(lang, dir, data).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render supplier profile", "error", err)
	}
}

// SupplierFollowSubmit toggles following for the signed-in user.
func (h *UIHandler) SupplierFollowSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, err := authctx.UserID(ctx)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect="+r.Referer(), http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil && h.orgSvc != nil {
		_, _ = h.orgSvc.ToggleFollow(ctx, id, userID)
	}

	back := r.Referer()
	if back == "" {
		back = "/suppliers"
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

// SupplierQuoteSubmit creates a bulk quote request addressed to a supplier.
func (h *UIHandler) SupplierQuoteSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect="+r.Referer(), http.StatusSeeOther)
		return
	}
	if actor.OrganizationID <= 0 {
		h.redirectWithNotice(w, r, "/suppliers", "error", "تحتاج إلى حساب مؤسسة معتمد لطلب عرض سعر.")
		return
	}

	supplierID, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	qty, _ := strconv.Atoi(r.PostFormValue("quantity"))
	if qty <= 0 {
		h.redirectWithNotice(w, r, "/suppliers/"+strconv.FormatInt(supplierID, 10), "error", "أدخل كمية صحيحة.")
		return
	}

	if h.commSvc == nil {
		h.redirectWithNotice(w, r, "/suppliers/"+strconv.FormatInt(supplierID, 10), "error", "الخدمة غير متاحة حالياً.")
		return
	}

	var productID *int64
	if pid, err := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64); err == nil && pid > 0 {
		productID = &pid
	}

	_, err := h.commSvc.CreateQuoteRequest(ctx, &commerce.QuoteRequest{
		OrganizationID:    supplierID,
		CustomerOrgID:     actor.OrganizationID,
		ProductID:         productID,
		RequestedQuantity: qty,
		BuyerNotes:        r.PostFormValue("notes"),
	})
	if err != nil {
		h.redirectWithNotice(w, r, "/suppliers/"+strconv.FormatInt(supplierID, 10), "error", h.safeMessage(err, langOf(r)))
		return
	}
	h.redirectWithNotice(w, r, "/suppliers/"+strconv.FormatInt(supplierID, 10), "success", "تم إرسال طلب عرض السعر.")
}
