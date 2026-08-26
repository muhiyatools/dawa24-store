package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/muhiya/dawa24-store/internal/modules/ingest"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// loadImportReview fills the review table with pagination and filters.
func (h *UIHandler) loadImportReview(r *http.Request, view *pages.VendorImportView) {
	ctx := r.Context()
	limit := 25
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && (l == 10 || l == 25 || l == 50 || l == 100) {
		limit = l
	}
	filter := ingest.RowFilter{
		Outcome:    r.URL.Query().Get("outcome"),
		MatchLevel: r.URL.Query().Get("match"),
		Search:     r.URL.Query().Get("q"),
		SortBy:     r.URL.Query().Get("sort"),
		SortOrder:  r.URL.Query().Get("order"),
		Limit:      limit,
	}
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 1 {
		page = p
		filter.Offset = (page - 1) * filter.Limit
	}
	view.Filter = filter
	view.Page = page
	view.PerPage = limit

	rows, total, err := h.ingSvc.ImportRows(ctx, view.Session.PublicID, filter)
	if err != nil {
		h.log.WarnContext(ctx, "import review rows unavailable", "error", err)
		return
	}
	view.Rows, view.RowTotal = rows, total

	counts, err := h.ingSvc.ImportRowCounts(ctx, view.Session.PublicID)
	if err != nil {
		h.log.WarnContext(ctx, "import row counts unavailable", "error", err)
		return
	}
	view.RowCounts = counts
}

// VendorIngestRowUpdateSubmit updates a staged row's variant name, price, quantity, etc.
func (h *UIHandler) VendorIngestRowUpdateSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	rowIDStr := chi.URLParam(r, "rowID")
	rowID, err := strconv.ParseInt(rowIDStr, 10, 64)
	if err != nil || h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", "معرّف الصف غير صالح.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", "تعذر قراءة النموذج المرسل.")
		return
	}

	customVariantName := strings.TrimSpace(r.PostFormValue("custom_variant_name"))
	displayName := strings.TrimSpace(r.PostFormValue("display_name"))

	var pricePtr *float64
	if pStr := strings.TrimSpace(r.PostFormValue("price")); pStr != "" {
		if pVal, pErr := strconv.ParseFloat(pStr, 64); pErr == nil {
			pricePtr = &pVal
		}
	}

	var qtyPtr *int
	if qStr := strings.TrimSpace(r.PostFormValue("quantity")); qStr != "" {
		if qVal, qErr := strconv.Atoi(qStr); qErr == nil {
			qtyPtr = &qVal
		}
	}

	if err := h.ingSvc.UpdateStagedRow(ctx, publicID, rowID, displayName, customVariantName, pricePtr, qtyPtr, nil); err != nil {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "error", h.safeMessage(err, langOf(r)))
		return
	}

	h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "success", "تم تحديث بيانات الصنف بنجاح.")
}

// VendorIngestRowMatchSubmit manually assigns a master catalog product to a staged row.
func (h *UIHandler) VendorIngestRowMatchSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	rowIDStr := chi.URLParam(r, "rowID")
	rowID, err := strconv.ParseInt(rowIDStr, 10, 64)
	if err != nil || h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", "معرّف الصف غير صالح.")
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", "تعذر قراءة النموذج.")
		return
	}

	productID, _ := strconv.ParseInt(r.PostFormValue("product_id"), 10, 64)

	if err := h.ingSvc.AssignStagedRowMatch(ctx, publicID, rowID, productID); err != nil {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "error", h.safeMessage(err, langOf(r)))
		return
	}

	if productID > 0 {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "success", "تم ربط الصنف بالكتالوج المركزي بنجاح.")
	} else {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "success", "تم إلغاء ربط الصنف بالكتالوج.")
	}
}

// VendorIngestRowToggleSubmit toggles whether a staged row will be included or excluded from commit.
func (h *UIHandler) VendorIngestRowToggleSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	rowIDStr := chi.URLParam(r, "rowID")
	rowID, err := strconv.ParseInt(rowIDStr, 10, 64)
	if err != nil || h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", "معرّف الصف غير صالح.")
		return
	}

	_, err = h.ingSvc.ToggleStagedRowExclude(ctx, publicID, rowID)
	if err != nil {
		h.redirectWithNotice(w, r, buildReviewRedirect(publicID, r), "error", h.safeMessage(err, langOf(r)))
		return
	}

	http.Redirect(w, r, buildReviewRedirect(publicID, r), http.StatusSeeOther)
}

// VendorIngestCatalogSearchJSON returns JSON search results from master catalog for modal picker.
func (h *UIHandler) VendorIngestCatalogSearchJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if h.ingSvc == nil || q == "" {
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}

	products, err := h.ingSvc.SearchMasterCatalog(ctx, q)
	if err != nil {
		http.Error(w, `{"error":"search_failed"}`, http.StatusInternalServerError)
		return
	}

	type searchItem struct {
		ID            int64   `json:"id"`
		NameAR        string  `json:"name_ar"`
		NameEN        string  `json:"name_en"`
		SKU           string  `json:"sku"`
		Barcode       string  `json:"barcode"`
		DosageForm    string  `json:"dosage_form"`
		Concentration string  `json:"concentration"`
		Manufacturer  string  `json:"manufacturer"`
		PublicPrice   float64 `json:"public_price"`
	}

	out := make([]searchItem, 0, len(products))
	for _, p := range products {
		nameAR := p.Name.Get("ar")
		nameEN := p.Name.Get("en")
		out = append(out, searchItem{
			ID:            p.ID,
			NameAR:        nameAR,
			NameEN:        nameEN,
			SKU:           p.SKU,
			Barcode:       p.Barcode,
			DosageForm:    p.DosageForm,
			Concentration: p.Concentration,
			Manufacturer:  p.ManufacturingCompanies,
			PublicPrice:   float64(p.Price.Minor()) / 100.0,
		})
	}

	_ = json.NewEncoder(w).Encode(out)
}

// VendorIngestBackToSettingsSubmit moves session back to settings phase.
func (h *UIHandler) VendorIngestBackToSettingsSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "خدمة الاستيراد غير متاحة حالياً.")
		return
	}

	if _, err := h.ingSvc.BackToSettings(ctx, publicID); err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", h.safeMessage(err, langOf(r)))
		return
	}

	http.Redirect(w, r, "/vendor/ingest/"+publicID, http.StatusSeeOther)
}

// VendorIngestCommitSubmit applies staged rows to the database.
func (h *UIHandler) VendorIngestCommitSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	publicID := chi.URLParam(r, "id")
	if h.ingSvc == nil {
		h.redirectWithNotice(w, r, "/vendor/ingest", "error", "خدمة الاستيراد غير متاحة حالياً.")
		return
	}

	session, err := h.ingSvc.CommitImport(ctx, publicID)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "error", h.safeMessage(err, langOf(r)))
		return
	}

	msg := fmt.Sprintf("تم بنجاح حفظ %d صنفاً في الكتالوج والمخزن.", session.InsertedRows+session.UpdatedRows)
	h.redirectWithNotice(w, r, "/vendor/ingest/"+publicID, "success", msg)
}

func buildReviewRedirect(publicID string, r *http.Request) string {
	q := r.URL.Query().Get("q")
	match := r.URL.Query().Get("match")
	sort := r.URL.Query().Get("sort")
	order := r.URL.Query().Get("order")
	page := r.URL.Query().Get("page")
	limit := r.URL.Query().Get("limit")

	url := "/vendor/ingest/" + publicID
	params := []string{}
	if match != "" {
		params = append(params, "match="+match)
	}
	if sort != "" {
		params = append(params, "sort="+sort)
	}
	if order != "" {
		params = append(params, "order="+order)
	}
	if page != "" {
		params = append(params, "page="+page)
	}
	if limit != "" {
		params = append(params, "limit="+limit)
	}
	if q != "" {
		params = append(params, "q="+q)
	}
	if len(params) > 0 {
		url += "?" + strings.Join(params, "&")
	}
	return url
}