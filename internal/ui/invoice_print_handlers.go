package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// InvoicePrintPage renders the Egyptian-standard printable invoice for a specific invoice ID.
func (h *UIHandler) InvoicePrintPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	var invoice *billing.Invoice
	if h.billSvc != nil {
		inv, errGet := h.billSvc.GetInvoice(ctx, id)
		if errGet == nil && inv != nil {
			invoice = inv
		}
	}

	if invoice == nil {
		http.NotFound(w, r)
		return
	}

	printableData, errBuild := h.buildPrintableInvoiceData(ctx, invoice, nil, lang)
	if errBuild != nil {
		h.log.ErrorContext(ctx, "failed to build printable invoice data", "error", errBuild)
		http.Error(w, i18n.T(lang, "invoice.prepare_failed"), http.StatusInternalServerError)
		return
	}

	h.renderPage(ctx, w, "render invoice print page", pages.InvoicePrintablePage(*printableData, lang, dir))
}

// OrderInvoicePrintPage renders the Egyptian-standard printable invoice for a specific order ID.
func (h *UIHandler) OrderInvoicePrintPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}

	var order *commerce.Order
	if h.commSvc != nil {
		ord, errGet := h.commSvc.GetOrder(ctx, id)
		if errGet == nil && ord != nil {
			order = ord
		}
	}

	if order == nil {
		http.NotFound(w, r)
		return
	}

	if order.Status == commerce.StatusPending {
		h.redirectWithNotice(w, r, fmt.Sprintf("/orders/%d", id), "error", i18n.T(lang, "invoice.print_pending_error"))
		return
	}

	printableData, errBuild := h.buildPrintableInvoiceData(ctx, nil, order, lang)
	if errBuild != nil {
		h.log.ErrorContext(ctx, "failed to build printable invoice data for order", "error", errBuild)
		http.Error(w, i18n.T(lang, "invoice.prepare_failed"), http.StatusInternalServerError)
		return
	}

	h.renderPage(ctx, w, "render order invoice print page", pages.InvoicePrintablePage(*printableData, lang, dir))
}
