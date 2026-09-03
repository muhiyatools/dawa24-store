package ui

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/muhiya/dawa24-store/internal/modules/billing"
	"github.com/muhiya/dawa24-store/internal/modules/commerce"
	"github.com/muhiya/dawa24-store/internal/platform/authctx"
	"github.com/muhiya/dawa24-store/internal/shared/money"
	"github.com/muhiya/dawa24-store/internal/shared/pagination"
	"github.com/muhiya/dawa24-store/internal/ui/pages"
)

// parseDateParam keeps only valid YYYY-MM-DD values so a malformed query can
// neither break the SQL layer nor silently filter everything out.
func parseDateParam(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return ""
	}
	return v
}

// VendorPaymentsPage renders the payments received by the vendor.
func (h *UIHandler) VendorPaymentsPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/payments", http.StatusSeeOther)
		return
	}

	search := strings.TrimSpace(r.URL.Query().Get("q"))
	method := strings.TrimSpace(r.URL.Query().Get("method"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	dateFrom := parseDateParam(r.URL.Query().Get("from"))
	dateTo := parseDateParam(r.URL.Query().Get("to"))

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var payments []*billing.AdminPaymentView
	var total int
	var stats *billing.VendorPaymentStats
	var openInvoices []*billing.AdminInvoiceView

	if h.billSvc != nil {
		s, err := h.billSvc.GetVendorPaymentStats(ctx, actor.OrganizationID)
		if err == nil {
			stats = s
		}

		filter := billing.PaymentFilter{
			OrganizationID: &actor.OrganizationID,
			Search:         search,
			Method:         method,
			Status:         status,
			DateFrom:       dateFrom,
			DateTo:         dateTo,
			Limit:          limit,
			Offset:         offset,
		}
		pList, count, err := h.billSvc.ListDetailedPayments(ctx, filter)
		if err == nil {
			payments = pList
			total = count
		}

		invList, _, _ := h.billSvc.ListDetailedInvoices(ctx, billing.InvoiceFilter{
			OrganizationID: &actor.OrganizationID,
			Limit:          100,
		})
		for _, inv := range invList {
			if inv.Status != billing.InvoicePaid && inv.Status != billing.InvoiceCancelled {
				openInvoices = append(openInvoices, inv)
			}
		}
	}

	if stats == nil {
		stats = &billing.VendorPaymentStats{}
	}

	data := pages.VendorPaymentsPageData{
		Payments:   payments,
		Invoices:   openInvoices,
		Stats:      stats,
		Search:     search,
		Method:     method,
		Status:     status,
		DateFrom:   dateFrom,
		DateTo:     dateTo,
		Page:       page,
		PerPage:    limit,
		TotalCount: total,
		Lang:       lang,
		Dir:        dir,
	}

	h.renderPage(ctx, w, "render vendor payments", pages.VendorPaymentsPage(data))
}

// VendorRecordPaymentSubmit records a payment against a vendor invoice.
func (h *UIHandler) VendorRecordPaymentSubmit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, _ := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/payments", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.redirectWithNotice(w, r, "/vendor/payments", "error", "تعذر قراءة بيانات النموذج")
		return
	}

	invoiceID, err := strconv.ParseInt(r.PostFormValue("invoice_id"), 10, 64)
	if err != nil || invoiceID <= 0 {
		h.redirectWithNotice(w, r, "/vendor/payments", "error", "يجب اختيار فاتورة صحيحة لتسجيل الدفعة عليها")
		return
	}

	amountStr := strings.TrimSpace(r.PostFormValue("amount"))
	amt, err := money.Parse(amountStr)
	if err != nil || amt.Minor() <= 0 {
		h.redirectWithNotice(w, r, "/vendor/payments", "error", "يرجى إدخال مبلغ دفع صالح أكبر من صفر")
		return
	}

	method := strings.TrimSpace(r.PostFormValue("method"))
	if method == "" {
		method = "bank_transfer"
	}

	refNum := strings.TrimSpace(r.PostFormValue("reference_number"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))

	req := billing.RecordInvoicePaymentRequest{
		InvoiceID:       invoiceID,
		OrganizationID:  actor.OrganizationID,
		UserID:          actor.UserID,
		Amount:          amt,
		Method:          method,
		ReferenceNumber: refNum,
		Notes:           notes,
	}

	_, err = h.billSvc.RecordInvoicePayment(ctx, req)
	if err != nil {
		h.redirectWithNotice(w, r, "/vendor/payments", "error", h.safeMessage(err, lang))
		return
	}

	h.redirectWithNotice(w, r, "/vendor/payments", "success", "تم تسجيل دفعة الفاتورة وتحديث الرصيد المتبقي بنجاح.")
}

// VendorEarningsOrderPage renders orders revenue and comprehensive net profit report for the vendor.
func (h *UIHandler) VendorEarningsOrderPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/earnings/order", http.StatusSeeOther)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	var summary *commerce.VendorFinancialSummary
	if h.commSvc != nil {
		summary, _ = h.commSvc.GetVendorFinancialSummary(ctx, actor.OrganizationID, period)
	}
	if summary == nil {
		summary = &commerce.VendorFinancialSummary{Period: period}
	}

	h.renderPage(ctx, w, "render vendor earnings order", pages.VendorEarningsOrderPage(summary, lang, dir))
}

// VendorEarningsOffersPage renders offers revenue and commissions for the vendor.
func (h *UIHandler) VendorEarningsOffersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/earnings/offers", http.StatusSeeOther)
		return
	}

	h.renderPage(ctx, w, "render vendor earnings offers", pages.VendorEarningsOffersPage(money.Zero, lang, dir))
}

// VendorOfferOrdersPage renders offer-based orders fulfilled by the vendor.
func (h *UIHandler) VendorOfferOrdersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login?redirect=/vendor/orders/offers", http.StatusSeeOther)
		return
	}

	limit := pagination.RowsPerPage(r)
	page := pagination.PageNumber(r)
	offset := (page - 1) * limit

	var shipments []*commerce.OrderShipment
	var total int
	if h.commSvc != nil {
		shipments, total, _ = h.commSvc.ListVendorShipmentsWithTotal(ctx, actor.OrganizationID, "", limit, offset)
	}

	h.renderPage(ctx, w, "render vendor offer orders", pages.VendorOfferOrdersPage(shipments, lang, dir, page, limit, total))
}

// VendorOfferOrderDetailPage renders single offer order details.
func (h *UIHandler) VendorOfferOrderDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang, dir := h.localeAndDir(r)

	actor, ok := authctx.From(ctx)
	if !ok || actor.OrganizationID <= 0 {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	idStr := chi.URLParam(r, "id")
	shipID, _ := strconv.ParseInt(idStr, 10, 64)

	h.renderPage(ctx, w, "render vendor offer order detail", pages.VendorOfferOrderDetailPage(shipID, lang, dir))
}
