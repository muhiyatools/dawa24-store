package ui

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
)

// Review-time actions.
//
// Each one edits the run and returns the buyer to the review screen, where the
// recomputed totals are visible immediately (FR-044). They are form posts rather
// than fetch calls so the screen works without JavaScript — a pharmacy on a slow
// connection with an old browser still has to be able to place an order.

// SetFinalizer wires the finalisation use case.
func (h *UIHandler) SetFinalizer(f *smartorder.Finalizer) { h.smartOrderFinalizer = f }

// SmartOrderQuantitySubmit applies a quantity edit.
func (h *UIHandler) SmartOrderQuantitySubmit(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	qty, err := strconv.ParseFloat(r.FormValue("quantity"), 64)
	if err != nil || qty < 0 {
		h.smartOrderBack(w, r, run, i18n.T(lang, "smartorder.invalid_quantity"))
		return
	}
	if err := h.smartOrderSvc.SetQuantity(r.Context(), run.OrganizationID, lineID, qty); err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err, lang))
		return
	}
	h.smartOrderRecalculate(r, run)
	h.smartOrderBack(w, r, run, "")
}

// SmartOrderSupplierSubmit switches a line to a different vendor.
func (h *UIHandler) SmartOrderSupplierSubmit(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	candidateID, err := strconv.ParseInt(r.FormValue("candidate_id"), 10, 64)
	if err != nil {
		h.smartOrderBack(w, r, run, i18n.T(lang, "smartorder.select_supplier_from_list"))
		return
	}
	if err := h.smartOrderSvc.ChooseSupplier(r.Context(), run.OrganizationID, lineID, candidateID); err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err, lang))
		return
	}
	h.smartOrderRecalculate(r, run)
	h.smartOrderBack(w, r, run, "")
}

// SmartOrderRemoveSubmit drops a line from the order.
func (h *UIHandler) SmartOrderRemoveSubmit(w http.ResponseWriter, r *http.Request) {
	lang := langOf(r)
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	if err := h.smartOrderSvc.RemoveLine(r.Context(), run.OrganizationID, lineID); err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err, lang))
		return
	}
	h.smartOrderRecalculate(r, run)
	h.smartOrderBack(w, r, run, "")
}

// SmartOrderFinalizeSubmit re-verifies every line and places the order.
//
// A line that changed since generation stops the whole order and is named on the
// review screen. Nothing is substituted and nothing is dropped: a pharmacy that
// ordered from one supplier and silently received from another finds out at
// delivery (FR-047).
func (h *UIHandler) SmartOrderFinalizeSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := langOf(r)
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}
	if h.smartOrderFinalizer == nil {
		h.smartOrderBack(w, r, run, i18n.T(lang, "smartorder.finalizer_unavailable"))
		return
	}

	orderID, stale, err := h.smartOrderFinalizer.Finalize(ctx, run)
	if err != nil {
		h.log.WarnContext(ctx, "smart order finalize failed", "run_id", run.ID, "error", err)
		h.smartOrderBack(w, r, run, translateSmartOrderError(err, lang))
		return
	}
	if len(stale) > 0 {
		// The review screen re-renders with the changed lines named. Storing
		// them on the run would outlive their usefulness; the buyer resolves
		// them now or re-runs.
		h.smartOrderStale.put(run.PublicID, stale)
		http.Redirect(w, r, "/customer/smart-order/"+run.PublicID+"/review", http.StatusSeeOther)
		return
	}

	h.smartOrderStale.drop(run.PublicID)
	http.Redirect(w, r, "/customer/orders/"+strconv.FormatInt(orderID, 10), http.StatusSeeOther)
}

// smartOrderLineAction resolves the run and line for a review edit.
func (h *UIHandler) smartOrderLineAction(w http.ResponseWriter, r *http.Request) (*smartorder.Run, int64, bool) {
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return nil, 0, false
	}
	if err := r.ParseForm(); err != nil {
		h.smartOrderBack(w, r, run, i18n.T(langOf(r), "smartorder.form_parse_error"))
		return nil, 0, false
	}
	lineID, err := strconv.ParseInt(chi.URLParam(r, "lineID"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return nil, 0, false
	}
	return run, lineID, true
}

// smartOrderRecalculate refreshes the run totals after an edit.
//
// A failure here is logged rather than surfaced: the edit itself succeeded, and
// the next page render recomputes anyway.
func (h *UIHandler) smartOrderRecalculate(r *http.Request, run *smartorder.Run) {
	ctx := r.Context()
	cfg, err := h.smartOrderSvc.Config(ctx, run.ID)
	if err != nil {
		h.log.WarnContext(ctx, "could not load config to recalculate", "run_id", run.ID, "error", err)
		return
	}
	if _, err := h.smartOrderSvc.Recalculate(ctx, run, cfg); err != nil {
		h.log.WarnContext(ctx, "could not recalculate smart order totals", "run_id", run.ID, "error", err)
	}
}

func (h *UIHandler) smartOrderBack(w http.ResponseWriter, r *http.Request, run *smartorder.Run, message string) {
	target := "/customer/smart-order/" + run.PublicID + "/review"
	if message != "" {
		target += "?error=" + url.QueryEscape(message)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

// SmartOrderCatalogSearch returns JSON results for the in-cell modal/combobox.
func (h *UIHandler) SmartOrderCatalogSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	if q == "" {
		_ = json.NewEncoder(w).Encode([]any{})
		return
	}

	type catalogItem struct {
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

	var out []catalogItem
	if h.ingSvc != nil {
		products, err := h.ingSvc.SearchMasterCatalog(ctx, q)
		if err == nil {
			for _, p := range products {
				out = append(out, catalogItem{
					ID:            p.ID,
					NameAR:        p.Name.Get("ar"),
					NameEN:        p.Name.Get("en"),
					SKU:           p.SKU,
					Barcode:       p.Barcode,
					DosageForm:    p.DosageForm,
					Concentration: p.Concentration,
					Manufacturer:  p.ManufacturingCompanies,
					PublicPrice:   float64(p.Price.Minor()) / 100.0,
				})
			}
		}
	}

	_ = json.NewEncoder(w).Encode(out)
}

// SmartOrderMatchSubmit updates or unmatches a line in smart order results.
func (h *UIHandler) SmartOrderMatchSubmit(w http.ResponseWriter, r *http.Request) {
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	_ = r.ParseForm()

	productID, _ := strconv.ParseInt(r.FormValue("product_id"), 10, 64)
	if h.smartOrderSvc != nil {
		if _, err := h.smartOrderSvc.CorrectMatch(r.Context(), run.OrganizationID, lineID, productID); err != nil {
			h.log.WarnContext(r.Context(), "smart order manual match failed", "line_id", lineID, "error", err)
		}
	}

	h.smartOrderRecalculate(r, run)

	q := r.URL.Query()
	vals := url.Values{}
	for _, key := range []string{"match", "outcome", "sort", "order", "page", "limit", "q"} {
		if v := q.Get(key); v != "" {
			vals.Set(key, v)
		}
	}
	redirectURL := "/customer/smart-order/" + run.PublicID + "/results"
	if len(vals) > 0 {
		redirectURL += "?" + vals.Encode()
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// RegisterSmartOrderRoutes mounts the wizard on the customer surface.
func (h *UIHandler) RegisterSmartOrderRoutes(r chi.Router) {
	r.Get("/customer/smart-order", h.SmartOrderHistoryPage)
	r.Get("/customer/smart-order/history", h.SmartOrderHistoryPage)
	r.Get("/customer/smart-order/new", h.SmartOrderNewPage)
	r.Post("/customer/smart-order", h.SmartOrderCreateSubmit)

	r.Get("/customer/smart-order/{id}/mapping", h.SmartOrderMappingPage)
	r.Post("/customer/smart-order/{id}/mapping", h.SmartOrderMappingSubmit)
	r.Get("/customer/smart-order/{id}/progress", h.SmartOrderProgressPage)
	r.Get("/customer/smart-order/{id}/progress.json", h.SmartOrderProgressJSON)
	r.Get("/customer/smart-order/{id}/results", h.SmartOrderResultsPage)
	r.Get("/customer/smart-order/{id}/catalog-search", h.SmartOrderCatalogSearch)
	r.Get("/customer/smart-order/{id}/review", h.SmartOrderReviewPage)
	r.Get("/customer/smart-order/{id}/export", h.SmartOrderExportCSV)

	r.Post("/customer/smart-order/{id}/lines/{lineID}/quantity", h.SmartOrderQuantitySubmit)
	r.Post("/customer/smart-order/{id}/lines/{lineID}/match", h.SmartOrderMatchSubmit)
	r.Post("/customer/smart-order/{id}/lines/{lineID}/supplier", h.SmartOrderSupplierSubmit)
	r.Post("/customer/smart-order/{id}/lines/{lineID}/remove", h.SmartOrderRemoveSubmit)
	r.Post("/customer/smart-order/{id}/finalize", h.SmartOrderFinalizeSubmit)
}
