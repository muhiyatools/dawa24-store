package ui

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/smartorder"
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
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	qty, err := strconv.ParseFloat(r.FormValue("quantity"), 64)
	if err != nil || qty < 0 {
		h.smartOrderBack(w, r, run, "أدخل كمية صحيحة.")
		return
	}
	if err := h.smartOrderSvc.SetQuantity(r.Context(), run.OrganizationID, lineID, qty); err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err))
		return
	}
	h.smartOrderRecalculate(r, run)
	h.smartOrderBack(w, r, run, "")
}

// SmartOrderSupplierSubmit switches a line to a different vendor.
func (h *UIHandler) SmartOrderSupplierSubmit(w http.ResponseWriter, r *http.Request) {
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	candidateID, err := strconv.ParseInt(r.FormValue("candidate_id"), 10, 64)
	if err != nil {
		h.smartOrderBack(w, r, run, "اختر موردًا من القائمة.")
		return
	}
	if err := h.smartOrderSvc.ChooseSupplier(r.Context(), run.OrganizationID, lineID, candidateID); err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err))
		return
	}
	h.smartOrderRecalculate(r, run)
	h.smartOrderBack(w, r, run, "")
}

// SmartOrderRemoveSubmit drops a line from the order.
func (h *UIHandler) SmartOrderRemoveSubmit(w http.ResponseWriter, r *http.Request) {
	run, lineID, ok := h.smartOrderLineAction(w, r)
	if !ok {
		return
	}
	if err := h.smartOrderSvc.RemoveLine(r.Context(), run.OrganizationID, lineID); err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err))
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
	run, ok := h.smartOrderRun(w, r)
	if !ok {
		return
	}
	if h.smartOrderFinalizer == nil {
		h.smartOrderBack(w, r, run, "اعتماد الطلبات غير مُفعّل حاليًا.")
		return
	}

	orderID, stale, err := h.smartOrderFinalizer.Finalize(ctx, run)
	if err != nil {
		h.smartOrderBack(w, r, run, translateSmartOrderError(err))
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
		h.smartOrderBack(w, r, run, "تعذّرت قراءة النموذج.")
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

// RegisterSmartOrderRoutes mounts the wizard on the customer surface.
//
// Registered alongside the existing purchase-request routes: smart ordering is
// an additional way to start a طلب الشراء, not a replacement for the ones
// already there (FR-000, FR-058).
func (h *UIHandler) RegisterSmartOrderRoutes(r chi.Router) {
	r.Get("/customer/smart-order", h.SmartOrderHistoryPage)
	r.Get("/customer/smart-order/history", h.SmartOrderHistoryPage)
	r.Get("/customer/smart-order/new", h.SmartOrderNewPage)
	r.Post("/customer/smart-order", h.SmartOrderCreateSubmit)

	r.Get("/customer/smart-order/{id}/mapping", h.SmartOrderMappingPage)
	r.Post("/customer/smart-order/{id}/mapping", h.SmartOrderMappingSubmit)
	r.Get("/customer/smart-order/{id}/progress", h.SmartOrderProgressPage)
	r.Get("/customer/smart-order/{id}/results", h.SmartOrderResultsPage)
	r.Get("/customer/smart-order/{id}/review", h.SmartOrderReviewPage)
	r.Get("/customer/smart-order/{id}/export", h.SmartOrderExportCSV)

	r.Post("/customer/smart-order/{id}/lines/{lineID}/quantity", h.SmartOrderQuantitySubmit)
	r.Post("/customer/smart-order/{id}/lines/{lineID}/supplier", h.SmartOrderSupplierSubmit)
	r.Post("/customer/smart-order/{id}/lines/{lineID}/remove", h.SmartOrderRemoveSubmit)
	r.Post("/customer/smart-order/{id}/finalize", h.SmartOrderFinalizeSubmit)
}
