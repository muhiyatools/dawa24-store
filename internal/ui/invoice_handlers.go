package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/modules/org"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/i18n"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// InvoicesPage renders the vendor's invoice list with status badges and actions.
func (h *UIHandler) InvoicesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok {
		http.Redirect(w, r, "/auth/login?redirect=/invoices", http.StatusSeeOther)
		return
	}

	// If pharmacy customer visits /invoices, redirect them to /orders since invoices are printed from order details
	if actor.IsCustomer() {
		h.redirectWithNotice(w, r, "/orders", "info", i18n.T(lang, "invoice.customer_redirect_info"))
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	branchIDStr := strings.TrimSpace(r.URL.Query().Get("branch_id"))
	var branchID *int64
	var selectedBranchID int64
	if bID, err := strconv.ParseInt(branchIDStr, 10, 64); err == nil && bID > 0 {
		branchID = &bID
		selectedBranchID = bID
	}

	page := pagination.PageNumber(r)
	limit := pagination.RowsPerPage(r)
	offset := (page - 1) * limit

	var orgID *int64
	if !actor.IsStaff && actor.OrganizationID > 0 {
		orgID = &actor.OrganizationID
	}

	var vendorBranches []*org.Branch
	if h.orgSvc != nil && actor.OrganizationID > 0 {
		if bList, err := h.orgSvc.ListBranches(ctx, actor.OrganizationID); err == nil {
			vendorBranches = bList
		}
	}

	var detailedInvoices []*billing.AdminInvoiceView
	var totalCount int
	if h.billSvc != nil {
		invoices, total, err := h.billSvc.AdminListDetailedInvoices(ctx, billing.InvoiceFilter{
			Search:         q,
			Status:         status,
			OrganizationID: orgID,
			BranchID:       branchID,
			Limit:          limit,
			Offset:         offset,
		})
		if err != nil {
			h.log.WarnContext(ctx, "account: list detailed invoices", "error", err)
		} else {
			detailedInvoices = invoices
			totalCount = total
		}
	}

	data := pages.InvoicesData{
		Invoices:         detailedInvoices,
		Search:           q,
		StatusFilter:     status,
		Branches:         vendorBranches,
		SelectedBranchID: selectedBranchID,
		IsVendor:         true,
		Page:             page,
		PerPage:          limit,
		TotalCount:       totalCount,
	}

	h.renderPage(ctx, w, "render invoices page", pages.InvoicesPage(lang, dir, data))
}

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
