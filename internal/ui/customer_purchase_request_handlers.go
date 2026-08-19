package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/catalog"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// CustomerPurchaseRequestWizardPage renders the multi-step purchase request wizard (Plan V5 Phase 3 §3.1.4).
func (h *UIHandler) CustomerPurchaseRequestWizardPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	stepStr := r.URL.Query().Get("step")
	step := 1
	if s, err := strconv.Atoi(stepStr); err == nil && s >= 1 && s <= 3 {
		step = s
	}

	option := r.URL.Query().Get("option") // "supplier" or "products"

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerPurchaseRequestWizardPage(lang, dir, step, option).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render purchase request wizard", "error", err)
	}
}

// CustomerPurchaseRequestSupplierPage lists suppliers with WithConnections institutional filter (Plan V5 §3.1.3).
func (h *UIHandler) CustomerPurchaseRequestSupplierPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, _ := authctx.From(ctx)

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// Fetch allowed suppliers under institutional works mode 2 (WithConnections)
	if h.orgSvc != nil && actor.UserID > 0 {
		_, _ = h.orgSvc.AllowedWorkIDs(ctx, actor.UserID, org.FilterWithConnections)
	}

	var suppliers []*org.Organization
	if h.orgSvc != nil {
		vType := org.TypeVendor
		vStatus := org.StatusApproved
		sups, err := h.orgSvc.ListOrganizations(ctx, &vType, &vStatus, 50, 0)
		if err == nil {
			if query != "" {
				var filtered []*org.Organization
				for _, s := range sups {
					if strings.Contains(strings.ToLower(s.LegalName), strings.ToLower(query)) ||
						strings.Contains(strings.ToLower(s.TradeName.Get("ar")), strings.ToLower(query)) {
						filtered = append(filtered, s)
					}
				}
				suppliers = filtered
			} else {
				suppliers = sups
			}
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerPurchaseRequestSupplierListPage(lang, dir, suppliers, query).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render purchase request supplier page", "error", err)
	}
}

// CustomerShowPurchaseRequestSupplierPage displays one supplier's catalog with WithConnections filter (Plan V5 §3.1.3).
func (h *UIHandler) CustomerShowPurchaseRequestSupplierPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, _ := authctx.From(ctx)

	supIDStr := chi.URLParam(r, "id")
	supID, err := strconv.ParseInt(supIDStr, 10, 64)
	if err != nil || supID <= 0 {
		http.Redirect(w, r, "/customer/purchase-request/supplier", http.StatusSeeOther)
		return
	}

	var supplier *org.Organization
	if h.orgSvc != nil {
		s, err := h.orgSvc.GetOrganization(ctx, supID)
		if err == nil {
			supplier = s
		}
	}
	if supplier == nil {
		http.Redirect(w, r, "/customer/purchase-request/supplier", http.StatusSeeOther)
		return
	}

	// Fetch supplier's products under institutional mode 2 (WithConnections)
	var allowedWorks []int64
	if h.orgSvc != nil && actor.UserID > 0 {
		if ids, err := h.orgSvc.AllowedWorkIDs(ctx, actor.UserID, org.FilterWithConnections); err == nil {
			allowedWorks = ids
		}
	}

	var products []*catalog.Product
	if h.catSvc != nil {
		prods, err := h.catSvc.Search(ctx, catalog.SearchParams{
			OrganizationID: &supID,
			AllowedWorkIDs: allowedWorks,
			Limit:          50,
		})
		if err == nil {
			products = prods
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerShowPurchaseRequestSupplierPage(lang, dir, supplier, products).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render show supplier purchase request page", "error", err)
	}
}

// CustomerPurchaseRequestProductsPage provides cross-supplier product picker with WithConnections filter (Plan V5 §3.1.3).
func (h *UIHandler) CustomerPurchaseRequestProductsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, _ := authctx.From(ctx)

	query := strings.TrimSpace(r.URL.Query().Get("q"))

	var allowedWorks []int64
	if h.orgSvc != nil && actor.UserID > 0 {
		if ids, err := h.orgSvc.AllowedWorkIDs(ctx, actor.UserID, org.FilterWithConnections); err == nil {
			allowedWorks = ids
		}
	}

	var products []*catalog.Product
	if h.catSvc != nil {
		prods, err := h.catSvc.Search(ctx, catalog.SearchParams{
			Query:          query,
			AllowedWorkIDs: allowedWorks,
			Limit:          50,
		})
		if err == nil {
			products = prods
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerPurchaseRequestProductsPage(lang, dir, products, query).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render purchase request products page", "error", err)
	}
}

// CustomerPurchaseRequestPreviousPage renders customer's submitted purchase requests with status tabs (Plan V5 §3.1.4).
func (h *UIHandler) CustomerPurchaseRequestPreviousPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/purchase-request/previous", http.StatusSeeOther)
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "all"
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	var requests []*commerce.PurchaseRequest
	var counts map[string]int
	if h.commSvc != nil {
		reqs, err := h.commSvc.ListCustomerPurchaseRequests(ctx, actor.UserID, orgPtr, status, 50, 0)
		if err == nil {
			requests = reqs
		}
		cnts, err := h.commSvc.CountCustomerPurchaseRequests(ctx, actor.UserID, orgPtr)
		if err == nil {
			counts = cnts
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pages.CustomerPurchaseRequestPreviousPage(lang, dir, requests, counts, status).Render(ctx, w); err != nil {
		h.log.ErrorContext(ctx, "render previous purchase requests page", "error", err)
	}
}

// CustomerPurchaseRequestSubmit handles final creation of purchase request with items (Plan V5 §3.1.1).
func (h *UIHandler) CustomerPurchaseRequestSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/customer/purchase-request", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/customer/purchase-request", "error", "تعذر قراءة بيانات النموذج.")
		return
	}

	vendorOrgID, err := strconv.ParseInt(r.FormValue("vendor_org_id"), 10, 64)
	if err != nil || vendorOrgID <= 0 {
		h.redirectWithNotice(w, r, "/customer/purchase-request", "error", "يرجى اختيار المورد المستهدف.")
		return
	}

	productNames := r.Form["product_name[]"]
	productIDs := r.Form["product_id[]"]
	skus := r.Form["sku[]"]
	quantities := r.Form["quantity[]"]
	targetPrices := r.Form["target_price[]"]
	targetDiscounts := r.Form["target_discount[]"]
	notes := r.Form["notes[]"]

	if len(productNames) == 0 {
		h.redirectWithNotice(w, r, "/customer/purchase-request", "error", "يرجى إضافة صنف واحد على الأقل للطلب.")
		return
	}

	var lines []*commerce.PurchaseRequestLine
	for i, name := range productNames {
		cleanName := strings.TrimSpace(name)
		if cleanName == "" {
			continue
		}

		qty := 0
		if i < len(quantities) {
			if q, err := strconv.Atoi(quantities[i]); err == nil && q > 0 {
				qty = q
			}
		}
		if qty <= 0 {
			continue
		}

		var pID *int64
		if i < len(productIDs) && productIDs[i] != "" {
			if id, err := strconv.ParseInt(productIDs[i], 10, 64); err == nil && id > 0 {
				pID = &id
			}
		}

		sku := ""
		if i < len(skus) {
			sku = strings.TrimSpace(skus[i])
		}

		var tPrice money.Amount
		if i < len(targetPrices) && targetPrices[i] != "" {
			if p, err := money.Parse(targetPrices[i]); err == nil {
				tPrice = p
			}
		}

		var tDisc float64
		if i < len(targetDiscounts) && targetDiscounts[i] != "" {
			if d, err := strconv.ParseFloat(targetDiscounts[i], 64); err == nil && d >= 0 {
				tDisc = d
			}
		}

		lineNote := ""
		if i < len(notes) {
			lineNote = strings.TrimSpace(notes[i])
		}

		lines = append(lines, &commerce.PurchaseRequestLine{
			ProductID:      pID,
			ProductName:    cleanName,
			ProductSKU:     sku,
			Quantity:       qty,
			TargetPrice:    tPrice,
			TargetDiscount: tDisc,
			Notes:          lineNote,
		})
	}

	if len(lines) == 0 {
		h.redirectWithNotice(w, r, "/customer/purchase-request", "error", "يرجى إدخال كمية أكبر من صفر للأصناف المطلوبة.")
		return
	}

	var orgPtr *int64
	if actor.OrganizationID > 0 {
		orgPtr = &actor.OrganizationID
	}

	buyerNotes := strings.TrimSpace(r.FormValue("buyer_notes"))

	req := &commerce.PurchaseRequest{
		CustomerID:     actor.UserID,
		OrganizationID: orgPtr,
		VendorOrgID:    vendorOrgID,
		BuyerNotes:     buyerNotes,
	}

	if h.commSvc != nil {
		created, err := h.commSvc.CreatePurchaseRequest(ctx, req, lines)
		if err != nil {
			h.redirectWithNotice(w, r, "/customer/purchase-request", "error", h.safeMessage(err, langOf(r)))
			return
		}
		h.redirectWithNotice(w, r, "/customer/purchase-request/previous", "success", fmt.Sprintf("تم إرسال طلب الشراء رقم %s بنجاح للمورد.", created.RequestNumber))
		return
	}

	h.redirectWithNotice(w, r, "/customer/purchase-request/previous", "success", "تم إرسال طلب الشراء بنجاح.")
}

// CustomerPurchaseRequestEditLineSubmit allows buyer to adjust target price/quantity on a pending purchase request.
func (h *UIHandler) CustomerPurchaseRequestEditLineSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if _, ok := authctx.From(ctx); !ok {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	lineIDStr := chi.URLParam(r, "id")
	lineID, err := strconv.ParseInt(lineIDStr, 10, 64)
	if err != nil || lineID <= 0 {
		h.redirectWithNotice(w, r, "/customer/purchase-request/previous", "error", "معرف سطر غير صالح.")
		return
	}

	var price money.Amount
	if pStr := r.FormValue("price"); pStr != "" {
		if p, err := money.Parse(pStr); err == nil {
			price = p
		}
	}

	var discount float64
	if dStr := r.FormValue("discount"); dStr != "" {
		if d, err := strconv.ParseFloat(dStr, 64); err == nil {
			discount = d
		}
	}

	if h.commSvc != nil {
		if err := h.commSvc.UpdatePurchaseRequestLineOffer(ctx, lineID, price, discount, "pending"); err != nil {
			h.redirectWithNotice(w, r, "/customer/purchase-request/previous", "error", h.safeMessage(err, langOf(r)))
			return
		}
	}

	h.redirectWithNotice(w, r, "/customer/purchase-request/previous", "success", "تم تعديل السطر بنجاح.")
}
